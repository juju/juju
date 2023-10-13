// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// UpgradeService is the interface for the upgrade service.
type UpgradeService interface {
	// CreateUpgrade creates an upgrade to and from specified versions
	// If an upgrade is already running/pending, return an AlreadyExists err
	CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (domainupgrade.UUID, error)
	// SetControllerReady marks the supplied controllerID as being ready
	// to start its upgrade. All provisioned controllers need to be ready
	// before an upgrade can start
	SetControllerReady(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// StartUpgrade starts the current upgrade if it exists
	StartUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// SetControllerDone marks the supplied controllerID as having
	// completed its upgrades. When SetControllerDone is called by the
	// last provisioned controller, the upgrade will be archived.
	SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// SetDBUpgradeCompleted marks the upgrade as completed in the database
	SetDBUpgradeCompleted(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// ActiveUpgrade returns the uuid of the current active upgrade.
	// If there are no active upgrades, return a NotFound error
	ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error)
	// WatchForUpgradeReady creates a watcher which notifies when all controller
	// nodes have been registered, meaning the upgrade is ready to start.
	WatchForUpgradeReady(ctx context.Context, upgradeUUID domainupgrade.UUID) (watcher.NotifyWatcher, error)
	// WatchForUpgradeState creates a watcher which notifies when the upgrade
	// has reached the given state.
	WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error)
}

// NewLock returns a new gate.Lock that is unlocked if the agent has not the same version as juju
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion().ToPatch()
	currentVersion := jujuversion.Current.ToPatch()

	if upgradedToVersion == currentVersion {
		lock.Unlock()
	}

	return lock
}

// Config holds the configuration for the worker.
type Config struct {
	// DBUpgradeCompleteLock is a lock used to synchronise workers that must
	// start after database upgrades are verified as completed.
	DBUpgradeCompleteLock gate.Lock

	// Agent is the running machine agent.
	Agent agent.Agent

	// UpgradeService is the upgrade service used to drive the upgrade.
	UpgradeService UpgradeService

	// DBGetter is the database getter used to get the database for each model.
	DBGetter coredatabase.DBGetter

	// Versions of the source and destination.
	FromVersion version.Number
	ToVersion   version.Number

	// Logger is the logger for this worker.
	Logger Logger
}

// Validate validates the worker configuration.
func (c Config) Validate() error {
	if c.DBUpgradeCompleteLock == nil {
		return errors.NotValidf("nil DBUpgradeCompleteLock")
	}
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.FromVersion == version.Zero {
		return errors.NotValidf("invalid FromVersion")
	}
	if c.ToVersion == version.Zero {
		return errors.NotValidf("invalid ToVersion")
	}
	return nil
}

type upgradeDBWorker struct {
	catacomb catacomb.Catacomb

	cfg Config

	service UpgradeService
}

// NewUpgradeDatabaseWorker returns a new Worker.
func NewUpgradeDatabaseWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeDBWorker{
		cfg:     config,
		service: config.UpgradeService,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill implements worker.Worker.Kill.
func (w *upgradeDBWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *upgradeDBWorker) Wait() error {
	return w.catacomb.Wait()
}

// loop implements Worker main loop.
func (w *upgradeDBWorker) loop() error {
	if w.upgradeDone() {
		// We're already upgraded, so we can uninstall this worker. This will
		// prevent it from running again, without an agent restart.
		return dependency.ErrUninstall
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	fromVersion := w.cfg.FromVersion
	toVersion := w.cfg.ToVersion

	// Create an upgrade for this controller. If another controller has already
	// created the upgrade, we will get an ErrUpgradeAlreadyStarted error. The
	// job of this controller is just to wait for the upgrade to be done and
	// then unlock the DBUpgradeCompleteLock.
	upgradeUUID, err := w.service.CreateUpgrade(ctx, fromVersion, toVersion)
	if err != nil {
		if errors.Is(err, upgradeerrors.ErrUpgradeAlreadyStarted) {
			// We're already running the upgrade, so we can just watch the
			// upgrade and wait for it to complete.
			return w.watchUpgrade()
		}
		return errors.Annotatef(err, "cannot create upgrade from: %v to: %v", fromVersion, toVersion)
	}

	return w.runUpgrade(upgradeUUID)
}

// watchUpgrade watches the upgrade until it is complete.
// Once the upgrade is complete, the DBUpgradeCompleteLock is unlocked.
func (w *upgradeDBWorker) watchUpgrade() error {
	w.cfg.Logger.Infof("watching upgrade")

	ctx, cancel := w.scopedContext()
	defer cancel()

	modelUUID, err := w.service.ActiveUpgrade(ctx)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// This currently no active upgrade, so we can't watch anything.
			// If this happens, it's probably in a bad state. We can't really
			// do anything about it, so we'll just bounce and hope that we
			// see if we've performed the upgrade already and that
			// we just didn't know about it in time.
			return dependency.ErrBounce
		}
		return errors.Trace(err)
	}

	watcher, err := w.service.WatchForUpgradeState(ctx, modelUUID, upgrade.DBCompleted)
	if err != nil {
		return errors.Annotate(err, "cannot watch upgrade")
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			// The upgrade is complete, so we can unlock the lock.
			w.cfg.Logger.Infof("database upgrade complete")
			w.cfg.DBUpgradeCompleteLock.Unlock()
			return dependency.ErrUninstall
		}
	}
}

// upgradeDone returns true if this worker does not need to run any upgrade
// logic.
func (w *upgradeDBWorker) upgradeDone() bool {
	// If we are already unlocked, there is nothing to do.
	if w.cfg.DBUpgradeCompleteLock.IsUnlocked() {
		return true
	}

	fromVersion := w.cfg.FromVersion
	toVersion := w.cfg.ToVersion
	if fromVersion == toVersion {
		w.cfg.Logger.Infof("database upgrade for %v already completed", toVersion)
		w.cfg.DBUpgradeCompleteLock.Unlock()
		return true
	}

	return false
}

func (w *upgradeDBWorker) runUpgrade(modelUUID domainupgrade.UUID) error {
	w.cfg.Logger.Infof("running database upgrade database")

	// This dummy worker, so we can just Unlock...
	w.cfg.Logger.Infof("database upgrade already completed")
	w.cfg.DBUpgradeCompleteLock.Unlock()

	return nil
}

func (w *upgradeDBWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
