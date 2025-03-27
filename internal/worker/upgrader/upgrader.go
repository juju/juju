// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/logger"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	jujuhttp "github.com/juju/juju/internal/http"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/worker/gate"
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

// UpgraderClient provides the facade methods used by the worker.
type UpgraderClient interface {
	DesiredVersion(ctx context.Context, tag string) (semversion.Number, error)
	SetVersion(ctx context.Context, tag string, v semversion.Binary) error
	WatchAPIVersion(ctx context.Context, agentTag string) (watcher.NotifyWatcher, error)
	Tools(ctx context.Context, tag string) (coretools.List, error)
}

// Upgrader represents a worker that watches the state for upgrade
// requests.
type Upgrader struct {
	catacomb catacomb.Catacomb
	client   UpgraderClient
	dataDir  string
	tag      names.Tag
	config   Config
}

// Config contains the items the worker needs to start.
type Config struct {
	Clock                       clock.Clock
	Logger                      logger.Logger
	Client                      UpgraderClient
	AgentConfig                 agent.Config
	OrigAgentVersion            semversion.Number
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
		client:  config.Client,
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
	curVersion semversion.Number,
	targetVersion semversion.Number,
) bool {
	// Don't allow downgrading from higher versions to version 1.x
	if curVersion.Major >= 2 && targetVersion.Major == 1 {
		return false
	}
	// Don't allow downgrading from higher major versions.
	return curVersion.Major <= targetVersion.Major
}

func (u *Upgrader) loop() error {
	ctx, cancel := u.scopedContext()
	defer cancel()

	logger := u.config.Logger
	// Start by reporting current tools (which includes arch/os type, and is
	// used by the controller in communicating the desired version below).
	hostOSType := coreos.HostOSTypeName()
	if err := u.client.SetVersion(ctx, u.tag.String(), toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
		return errors.Annotatef(err, "setting agent version for %q", u.tag.String())
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
		u.config.Logger.Infof(ctx, "no waiter, upgrader is done")
		return nil
	}

	versionWatcher, err := u.client.WatchAPIVersion(ctx, u.tag.String())
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

		wantVersion, err := u.client.DesiredVersion(ctx, u.tag.String())
		if err != nil {
			return err
		}
		logger.Infof(ctx, "desired agent binary version: %v", wantVersion)

		// If we have a desired version of Juju without the build number,
		// i.e. it is not a user compiled version, reset the build number of
		// the current version to remove the Jenkins build number.
		// We don't care about the build number when checking for upgrade.
		haveVersion := jujuversion.Current
		if wantVersion.Build == 0 {
			haveVersion.Build = 0
		}

		if wantVersion == haveVersion {
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		} else if !AllowedTargetVersion(haveVersion, wantVersion) {
			logger.Infof(ctx, "downgrade from %v to %v is not possible", haveVersion, wantVersion)
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		}
		direction := "upgrade"
		if wantVersion.Compare(haveVersion) == -1 {
			direction = "downgrade"
		}
		logger.Infof(ctx, "%s requested from %v to %v", direction, haveVersion, wantVersion)

		// Check if tools have already been downloaded.
		wantVersionBinary := toBinaryVersion(wantVersion, hostOSType)
		if u.toolsAlreadyDownloaded(wantVersionBinary) {
			return u.newUpgradeReadyError(haveVersion, wantVersionBinary, hostOSType)
		}

		// Check if tools are available for download.
		wantToolsList, err := u.client.Tools(ctx, u.tag.String())
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
			if err := u.checkForSpace(ctx); err != nil {
				logger.Errorf(ctx, "%s", err.Error())
				delay = notEnoughSpaceDelay
				break
			}
			err = u.ensureTools(ctx, wantTools)
			if err == nil {
				return u.newUpgradeReadyError(haveVersion, wantTools.Version, hostOSType)
			}
			logger.Errorf(ctx, "failed to fetch agent binaries from %q: %v", wantTools.URL, err)
		}
		retry = u.config.Clock.After(delay)
	}
}

func toBinaryVersion(vers semversion.Number, osType string) semversion.Binary {
	outVers := semversion.Binary{
		Number:  vers,
		Arch:    arch.HostArch(),
		Release: osType,
	}
	return outVers
}

func (u *Upgrader) toolsAlreadyDownloaded(wantVersion semversion.Binary) bool {
	_, err := agenttools.ReadTools(u.dataDir, wantVersion)
	return err == nil
}

func (u *Upgrader) newUpgradeReadyError(haveVersion semversion.Number, newVersion semversion.Binary, osType string) *agenterrors.UpgradeReadyError {
	return &agenterrors.UpgradeReadyError{
		OldTools:  toBinaryVersion(haveVersion, osType),
		NewTools:  newVersion,
		AgentName: u.tag.String(),
		DataDir:   u.dataDir,
	}
}

func (u *Upgrader) ensureTools(ctx context.Context, agentTools *coretools.Tools) error {
	u.config.Logger.Infof(ctx, "fetching agent binaries from %q", agentTools.URL)
	// The reader MUST verify the tools' hash, so there is no
	// need to validate the peer. We cannot anyway: see http://pad.lv/1261780.
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	resp, err := client.Get(ctx, agentTools.URL)
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
	u.config.Logger.Infof(ctx, "unpacked agent binaries %s to %s", agentTools.Version, u.dataDir)
	return nil
}

func (u *Upgrader) checkForSpace(ctx context.Context) error {
	u.config.Logger.Debugf(ctx, "checking available space before downloading")
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

func (u *Upgrader) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(u.catacomb.Context(context.Background()))
}
