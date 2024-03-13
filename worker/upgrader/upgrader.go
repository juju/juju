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
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/agent/upgrader"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/arch"
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
	// Don't allow downgrading from higher major versions.
	return curVersion.Major <= targetVersion.Major
}

func (u *Upgrader) loop() error {
	logger := u.config.Logger
	// Start by reporting current tools (which includes arch/os type, and is
	// used by the controller in communicating the desired version below).
	hostOSType := coreos.HostOSTypeName()
	if err := u.st.SetVersion(u.tag.String(), toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
		return errors.Annotatef(err, "cannot set agent version for %q", u.tag.String())
	}

	// We do not commence any actions until the upgrade-steps worker has
	// confirmed that all steps are completed for getting us upgraded to the
	// version that we currently on.
	if u.config.UpgradeStepsWaiter != nil {
		select {
		case <-u.config.UpgradeStepsWaiter.Unlocked():
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		}
	}

	if u.config.UpgradeStepsWaiter == nil {
		u.config.Logger.Infof("no waiter, upgrader is done")
		return nil
	}

	versionWatcher, err := u.st.WatchAPIVersion(u.tag.String())
	if err != nil {
		return errors.Trace(err)
	}

	var retry <-chan time.Time
	for {
		select {
		case <-retry:
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case _, ok := <-versionWatcher.Changes():
			if !ok {
				return errors.New("version watcher closed")
			}
		}

		haveVersion := jujuversion.Current
		wantVersion, err := u.st.DesiredVersion(u.tag.String())
		if err != nil {
			return err
		}
		logger.Infof("desired agent binary version: %v", wantVersion)

		if wantVersion == haveVersion {
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		} else if !AllowedTargetVersion(haveVersion, wantVersion) {
			logger.Infof("downgrade from %v to %v is not possible", haveVersion, wantVersion)
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
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
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
