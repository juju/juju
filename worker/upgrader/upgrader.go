// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http"
	"github.com/juju/names/v4"
	"github.com/juju/os/v2/series"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/utils/v2/fs"
	"github.com/juju/utils/v2/symlink"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/upgrader"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	coreos "github.com/juju/juju/core/os"
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

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
type logger interface{}

var _ logger = struct{}{}

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
	Clock                       Clock
	Logger                      Logger
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

// AllowedTargetVersion checks if targetVersion is too different from
// curVersion to allow a downgrade.
func AllowedTargetVersion(
	curVersion version.Number,
	targetVersion version.Number,
) bool {
	// Don't allow downgrading from higher versions to version 1.x
	if curVersion.Major >= 2 && targetVersion.Major == 1 {
		return false
	}
	return true
}

func (u *Upgrader) maybeCopyAgentBinary(dataDir, hostSeries string) (err error) {
	// Only need to rewrite tools symlink for older agents.
	if u.config.OrigAgentVersion.Major > 2 || u.config.OrigAgentVersion.Minor > 8 {
		return nil
	}

	toVer := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	// See if we already have what we want.
	if _, err = os.Stat(agenttools.SharedToolsDir(dataDir, toVer)); err == nil {
		return nil
	}

	fromVer := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: hostSeries,
	}
	// If the old tools have already been removed, there's nothing we really need to do.
	if _, err = os.Stat(agenttools.SharedToolsDir(dataDir, fromVer)); err != nil {
		return nil
	}

	// Copy tools to new directory with correct release string.
	if err = fs.Copy(agenttools.SharedToolsDir(dataDir, fromVer), agenttools.SharedToolsDir(dataDir, toVer)); err != nil {
		return errors.Trace(err)
	}

	// Write tools metadata with new version, however don't change
	// the URL, so we know where it came from.
	jujuTools, err := agenttools.ReadTools(dataDir, toVer)
	if err != nil {
		return errors.Trace(err)
	}

	// Only write once
	if jujuTools.Version != toVer {
		jujuTools.Version = toVer
		if err = agenttools.WriteToolsMetadataData(agenttools.ToolsDir(dataDir, toVer.String()), jujuTools); err != nil {
			return errors.Trace(err)
		}
	}

	// Update machine agent tool link.
	toolPath := agenttools.ToolsDir(dataDir, toVer.String())
	toolsDir := agenttools.ToolsDir(dataDir, u.tag.String())

	if err = symlink.Replace(toolsDir, toolPath); err != nil {
		return errors.Trace(err)
	}

	return &agenterrors.UpgradeReadyError{
		OldTools:  fromVer,
		NewTools:  toVer,
		AgentName: u.tag.String(),
		DataDir:   u.dataDir,
	}
}

func (u *Upgrader) loop() error {
	logger := u.config.Logger
	// Start by reporting current tools (which includes arch/os type, and is
	// used by the controller in communicating the desired version below).
	hostOSType := coreos.HostOSTypeName()
	if err := u.st.SetVersion(u.tag.String(), toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
		return errors.Annotate(err, "cannot set agent version")
	}

	// Older 2.8 agents still use for tools based on series.
	// Old upgrade the original agent will write the tool symlink
	// also based on series so we need to copy it across to one
	// based on OS type.
	// TODO(juju4) - remove this logic
	hostSeries, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.maybeCopyAgentBinary(u.dataDir, hostSeries); err != nil {
		// Do not wrap to preserve naked UpgradeReadyError.
		return err
	}

	if u.config.UpgradeStepsWaiter == nil {
		u.config.Logger.Infof("no waiter, upgrader is done")
		return nil
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
	mustProceed := u.config.Clock.After(time.Minute)
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

		// If we have a desired version of Juju without the build number, ie it is
		// not a user compiled version, reset the build number of the current version
		// to remove the Jenkins build number. We don't care about the Jenkins build
		// number when doing an upgrade check.
		haveVersion := jujuversion.Current
		if wantVersion.Build == 0 {
			haveVersion.Build = 0
		}

		if wantVersion == haveVersion {
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		} else if !AllowedTargetVersion(
			haveVersion,
			wantVersion,
		) {
			// Don't allow downgrading to v1.x - we don't support
			// restoring from a 1.x backup.
			logger.Infof("desired agent binary version: %s is older than 2.0.0, refusing to downgrade",
				wantVersion)
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		}
		direction := "upgrade"
		if wantVersion.Compare(haveVersion) == -1 {
			direction = "downgrade"
		}
		logger.Infof("%s requested from %v to %v", direction, haveVersion, wantVersion)

		// Check if tools have already been downloaded.
		wantVersionBinary := toBinaryVersion(wantVersion, hostOSType)
		if u.toolsAlreadyDownloaded(wantVersionBinary) {
			return u.newUpgradeReadyError(haveVersion, wantVersionBinary, hostOSType)
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
				return u.newUpgradeReadyError(haveVersion, wantTools.Version, hostOSType)
			}
			logger.Errorf("failed to fetch agent binaries from %q: %v", wantTools.URL, err)
		}
		retry = u.config.Clock.After(delay)
	}
}

func toBinaryVersion(vers version.Number, osType string) version.Binary {
	outVers := version.Binary{
		Number:  vers,
		Arch:    arch.HostArch(),
		Release: osType,
	}
	return outVers
}

func (u *Upgrader) toolsAlreadyDownloaded(wantVersion version.Binary) bool {
	_, err := agenttools.ReadTools(u.dataDir, wantVersion)
	return err == nil
}

func (u *Upgrader) newUpgradeReadyError(haveVersion version.Number, newVersion version.Binary, osType string) *agenterrors.UpgradeReadyError {
	return &agenterrors.UpgradeReadyError{
		OldTools:  toBinaryVersion(haveVersion, osType),
		NewTools:  newVersion,
		AgentName: u.tag.String(),
		DataDir:   u.dataDir,
	}
}

func (u *Upgrader) ensureTools(agentTools *coretools.Tools) error {
	u.config.Logger.Infof("fetching agent binaries from %q", agentTools.URL)
	// The reader MUST verify the tools' hash, so there is no
	// need to validate the peer. We cannot anyway: see http://pad.lv/1261780.
	client := jujuhttp.NewClient(jujuhttp.Config{SkipHostnameVerification: true})
	resp, err := client.Get(context.TODO(), agentTools.URL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad HTTP response: %v", resp.Status)
	}
	err = agenttools.UnpackTools(u.dataDir, agentTools, resp.Body)
	if err != nil {
		return fmt.Errorf("cannot unpack agent binaries: %v", err)
	}
	u.config.Logger.Infof("unpacked agent binaries %s to %s", agentTools.Version, u.dataDir)
	return nil
}

func (u *Upgrader) checkForSpace() error {
	u.config.Logger.Debugf("checking available space before downloading")
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
