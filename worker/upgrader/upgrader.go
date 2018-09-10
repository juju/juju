// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/upgrader"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

const (
	// shortDelay is the time we normally sleep for in the main loop
	// when polling for changes to the model's version.
	shortDelay = 5 * time.Second

	// notEnoughSpaceDelay is how long we sleep when there's a new
	// version of the agent that we need to download but there isn't
	// enough available space to download and unpack it. Sleeping
	// longer in that situation means we don't spam the log with disk
	// space errors every 3 seconds, but still bring the message up
	// regularly.
	notEnoughSpaceDelay = time.Minute
)

// retryAfter returns a channel that receives a value
// when a failed download should be retried.
var retryAfter = func(duration time.Duration) <-chan time.Time {
	// TODO(fwereade): 2016-03-17 lp:1558657
	return time.After(duration)
}

var logger = loggo.GetLogger("juju.worker.upgrader")

// Upgrader represents a worker that watches the state for upgrade
// requests.
type Upgrader struct {
	catacomb catacomb.Catacomb
	st       *upgrader.State
	dataDir  string
	tag      names.Tag
	config   Config
}

// Config contains the items the worker needs to start.
type Config struct {
	State                       *upgrader.State
	AgentConfig                 agent.Config
	OrigAgentVersion            version.Number
	UpgradeStepsWaiter          gate.Waiter
	InitialUpgradeCheckComplete gate.Unlocker
	CheckDiskSpace              func(string, uint64) error
}

// NewAgentUpgrader returns a new upgrader worker. It watches changes to the
// current version of the current agent (with the given tag) and tries to
// download the tools for any new version into the given data directory.  If
// an upgrade is needed, the worker will exit with an UpgradeReadyError
// holding details of the requested upgrade. The tools will have been
// downloaded and unpacked.
func NewAgentUpgrader(config Config) (*Upgrader, error) {
	u := &Upgrader{
		st:      config.State,
		dataDir: config.AgentConfig.DataDir(),
		tag:     config.AgentConfig.Tag(),
		config:  config,
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
	// Start by reporting current tools (which includes arch/series, and is
	// used by the controller in communicating the desired version below).
	if err := u.st.SetVersion(u.tag.String(), toBinaryVersion(jujuversion.Current)); err != nil {
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
		logger.Infof("desired agent binary version: %v", wantVersion)

		if wantVersion == jujuversion.Current {
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		} else if !allowedTargetVersion(
			u.config.OrigAgentVersion,
			jujuversion.Current,
			!u.config.UpgradeStepsWaiter.IsUnlocked(),
			wantVersion,
		) {
			// See also bug #1299802 where when upgrading from
			// 1.16 to 1.18 there is a race condition that can
			// cause the unit agent to upgrade, and then want to
			// downgrade when its associate machine agent has not
			// finished upgrading.
			logger.Infof("desired agent binary version: %s is older than current %s, refusing to downgrade",
				wantVersion, jujuversion.Current)
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		}
		logger.Infof("upgrade requested from %v to %v", jujuversion.Current, wantVersion)

		// Check if tools have already been downloaded.
		wantVersionBinary := toBinaryVersion(wantVersion)
		if u.toolsAlreadyDownloaded(wantVersionBinary) {
			return u.newUpgradeReadyError(wantVersionBinary)
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
		delay := shortDelay
		for _, wantTools := range wantToolsList {
			if err := u.checkForSpace(); err != nil {
				logger.Errorf("%s", err.Error())
				delay = notEnoughSpaceDelay
				break
			}
			err = u.ensureTools(wantTools)
			if err == nil {
				return u.newUpgradeReadyError(wantTools.Version)
			}
			logger.Errorf("failed to fetch agent binaries from %q: %v", wantTools.URL, err)
		}
		retry = retryAfter(delay)
	}
}

func toBinaryVersion(vers version.Number) version.Binary {
	outVers := version.Binary{
		Number: vers,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	return outVers
}

func (u *Upgrader) toolsAlreadyDownloaded(wantVersion version.Binary) bool {
	_, err := agenttools.ReadTools(u.dataDir, wantVersion)
	return err == nil
}

func (u *Upgrader) newUpgradeReadyError(newVersion version.Binary) *UpgradeReadyError {
	return &UpgradeReadyError{
		OldTools:  toBinaryVersion(jujuversion.Current),
		NewTools:  newVersion,
		AgentName: u.tag.String(),
		DataDir:   u.dataDir,
	}
}

func (u *Upgrader) ensureTools(agentTools *coretools.Tools) error {
	logger.Infof("fetching agent binaries from %q", agentTools.URL)
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
		return fmt.Errorf("cannot unpack agent binaries: %v", err)
	}
	logger.Infof("unpacked agent binaries %s to %s", agentTools.Version, u.dataDir)
	return nil
}

func (u *Upgrader) checkForSpace() error {
	logger.Debugf("checking available space before downloading")
	err := u.config.CheckDiskSpace(u.dataDir, upgrades.MinDiskSpaceMib)
	if err != nil {
		return errors.Trace(err)
	}
	err = u.config.CheckDiskSpace(os.TempDir(), upgrades.MinDiskSpaceMib)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
