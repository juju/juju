// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/upgrader"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/gate"
)

// retryAfter returns a channel that receives a value
// when a failed download should be retried.
var retryAfter = func() <-chan time.Time {
	// TODO(fwereade): 2016-03-17 lp:1558657
	return time.After(5 * time.Second)
}

var logger = loggo.GetLogger("juju.worker.upgrader")

// Upgrader represents a worker that watches the state for upgrade
// requests.
type Upgrader struct {
	catacomb                    catacomb.Catacomb
	st                          *upgrader.State
	dataDir                     string
	tag                         names.Tag
	origAgentVersion            version.Number
	upgradeStepsWaiter          gate.Waiter
	initialUpgradeCheckComplete gate.Unlocker
}

// NewAgentUpgrader returns a new upgrader worker. It watches changes to the
// current version of the current agent (with the given tag) and tries to
// download the tools for any new version into the given data directory.  If
// an upgrade is needed, the worker will exit with an UpgradeReadyError
// holding details of the requested upgrade. The tools will have been
// downloaded and unpacked.
func NewAgentUpgrader(
	st *upgrader.State,
	agentConfig agent.Config,
	origAgentVersion version.Number,
	upgradeStepsWaiter gate.Waiter,
	initialUpgradeCheckComplete gate.Unlocker,
) (*Upgrader, error) {
	u := &Upgrader{
		st:                          st,
		dataDir:                     agentConfig.DataDir(),
		tag:                         agentConfig.Tag(),
		origAgentVersion:            origAgentVersion,
		upgradeStepsWaiter:          upgradeStepsWaiter,
		initialUpgradeCheckComplete: initialUpgradeCheckComplete,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// Kill implements worker.Worker.Kill.
func (u *Upgrader) Kill() {
	u.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (u *Upgrader) Wait() error {
	return u.catacomb.Wait()
}

// Stop stops the upgrader and returns any
// error it encountered when running.
func (u *Upgrader) Stop() error {
	u.Kill()
	return u.Wait()
}

// allowedTargetVersion checks if targetVersion is too different from
// curVersion to allow a downgrade.
func allowedTargetVersion(
	origAgentVersion version.Number,
	curVersion version.Number,
	upgradeStepsRunning bool,
	targetVersion version.Number,
) bool {
	if upgradeStepsRunning && targetVersion == origAgentVersion {
		return true
	}
	if targetVersion.Major < curVersion.Major {
		return false
	}
	if targetVersion.Major == curVersion.Major && targetVersion.Minor < curVersion.Minor {
		return false
	}
	return true
}

func (u *Upgrader) loop() error {
	series, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}

	// Start by reporting current tools (which includes arch/series, and is
	// used by the controller in communicating the desired version below).
	if err := u.st.SetVersion(u.tag.String(), toBinaryVersion(series, jujuversion.Current)); err != nil {
		return errors.Annotate(err, "cannot set agent version")
	}

	// We don't read on the dying channel until we have received the
	// initial event from the API version watcher, thus ensuring
	// that we attempt an upgrade even if other workers are dying
	// all around us. Similarly, we don't want to bind the watcher
	// to the catacomb's lifetime (yet!) lest we wait forever for a
	// stopped watcher.
	//
	// However, that absolutely depends on versionWatcher's guaranteed
	// initial event, and we should assume that it'll break its contract
	// sometime. So we allow the watcher to wait patiently for the event
	// for a full minute; but after that we proceed regardless.
	versionWatcher, err := u.st.WatchAPIVersion(u.tag.String())
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("abort check blocked until version event received")
	// TODO(fwereade): 2016-03-17 lp:1558657
	mustProceed := time.After(time.Minute)
	var dying <-chan struct{}
	allowDying := func() {
		if dying == nil {
			logger.Infof("unblocking abort check")
			mustProceed = nil
			dying = u.catacomb.Dying()
			if err := u.catacomb.Add(versionWatcher); err != nil {
				u.catacomb.Kill(err)
			}
		}
	}

	var retry <-chan time.Time
	for {
		select {
		// NOTE: retry and dying both start out nil, so they can't be chosen
		// first time round the loop. However...
		case <-retry:
		case <-dying:
			return u.catacomb.ErrDying()
		// ...*every* other case *must* allowDying(), before doing anything
		// else, lest an error cause us to leak versionWatcher.
		case <-mustProceed:
			logger.Infof("version event not received after one minute")
			allowDying()
		case _, ok := <-versionWatcher.Changes():
			allowDying()
			if !ok {
				return errors.New("version watcher closed")
			}
		}

		wantVersion, err := u.st.DesiredVersion(u.tag.String())
		if err != nil {
			return err
		}
		logger.Infof("desired tool version: %v", wantVersion)

		if wantVersion == jujuversion.Current {
			u.initialUpgradeCheckComplete.Unlock()
			continue
		} else if !allowedTargetVersion(
			u.origAgentVersion,
			jujuversion.Current,
			!u.upgradeStepsWaiter.IsUnlocked(),
			wantVersion,
		) {
			// See also bug #1299802 where when upgrading from
			// 1.16 to 1.18 there is a race condition that can
			// cause the unit agent to upgrade, and then want to
			// downgrade when its associate machine agent has not
			// finished upgrading.
			logger.Infof("desired tool version: %s is older than current %s, refusing to downgrade",
				wantVersion, jujuversion.Current)
			u.initialUpgradeCheckComplete.Unlock()
			continue
		}
		logger.Infof("upgrade requested from %v to %v", jujuversion.Current, wantVersion)

		// Check if tools have already been downloaded.
		wantVersionBinary := toBinaryVersion(series, wantVersion)
		if u.toolsAlreadyDownloaded(wantVersionBinary) {
			return u.newUpgradeReadyError(series, wantVersionBinary)
		}

		// Check if tools are available for download.
		wantToolsList, err := u.st.Tools(u.tag.String())
		if err != nil {
			// Not being able to lookup Tools is considered fatal
			return err
		}
		// The worker cannot be stopped while we're downloading
		// the tools - this means that even if the API is going down
		// repeatedly (causing the agent to be stopped), as long
		// as we have got as far as this, we will still be able to
		// upgrade the agent.
		for _, wantTools := range wantToolsList {
			err = u.ensureTools(wantTools)
			if err == nil {
				return u.newUpgradeReadyError(series, wantTools.Version)
			}
			logger.Errorf("failed to fetch tools from %q: %v", wantTools.URL, err)
		}
		retry = retryAfter()
	}
}

func toBinaryVersion(series string, vers version.Number) version.Binary {
	return version.Binary{
		Number: vers,
		Arch:   arch.HostArch(),
		Series: series,
	}
}

func (u *Upgrader) toolsAlreadyDownloaded(wantVersion version.Binary) bool {
	_, err := agenttools.ReadTools(u.dataDir, wantVersion)
	return err == nil
}

func (u *Upgrader) newUpgradeReadyError(series string, newVersion version.Binary) *UpgradeReadyError {
	return &UpgradeReadyError{
		OldTools:  toBinaryVersion(series, jujuversion.Current),
		NewTools:  newVersion,
		AgentName: u.tag.String(),
		DataDir:   u.dataDir,
	}
}

func (u *Upgrader) ensureTools(agentTools *coretools.Tools) error {
	logger.Infof("fetching tools from %q", agentTools.URL)
	// The reader MUST verify the tools' hash, so there is no
	// need to validate the peer. We cannot anyway: see http://pad.lv/1261780.
	resp, err := utils.GetNonValidatingHTTPClient().Get(agentTools.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad HTTP response: %v", resp.Status)
	}
	err = agenttools.UnpackTools(u.dataDir, agentTools, resp.Body)
	if err != nil {
		return fmt.Errorf("cannot unpack tools: %v", err)
	}
	logger.Infof("unpacked tools %s to %s", agentTools.Version, u.dataDir)
	return nil
}
