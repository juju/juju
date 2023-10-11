// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	domainupgrade "github.com/juju/juju/domain/upgrade"
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
	// WatchForUpgradeStarted creates a watcher which notifies when all controller
	// nodes have been registered, meaning the upgrade is ready to start.
	WatchForUpgradeStarted(ctx context.Context, upgradeUUID domainupgrade.UUID) (watcher.NotifyWatcher, error)
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
	// UpgradeComplete is a lock used to synchronise workers that must start
	// after database upgrades are verified as completed.
	UpgradeCompleteLock gate.Lock

	// Agent is the running machine agent.
	Agent agent.Agent

	// UpgradeService is the upgrade service used to drive the upgrade.
	UpgradeService UpgradeService

	// DBGetter is the database getter used to get the database for each model.
	DBGetter coredatabase.DBGetter

	// Logger is the logger for this worker.
	Logger Logger
}

// Validate validates the worker configuration.
func (c Config) Validate() error {
	if c.UpgradeCompleteLock == nil {
		return errors.NotValidf("nil UpgradeCompleteLock")
	}
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type upgradeDBWorker struct {
	catacomb catacomb.Catacomb

	cfg Config

	fromVersion version.Number
	toVersion   version.Number
}

// NewUpgradeDatabaseWorker returns a new Worker.
func NewUpgradeDatabaseWorker(config Config) (*upgradeDBWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeDBWorker{
		cfg: config,
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
		return nil
	}

	// TODO(anvial): try to CreateUpgrade, the controller who gets the upgrade ID will rubUpgrade, all other controllers
	// should get the error and only watchUpgrade

	err := w.runUpgrade()
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// upgradeDone returns true if this worker
// does not need to run any upgrade logic.
func (w *upgradeDBWorker) upgradeDone() bool {
	// If we are already unlocked, there is nothing to do.
	if w.cfg.UpgradeCompleteLock.IsUnlocked() {
		return true
	}

	// If we are already on the current version, there is nothing to do.
	w.fromVersion = w.cfg.Agent.CurrentConfig().UpgradedToVersion()
	w.toVersion = jujuversion.Current
	if w.fromVersion == w.toVersion {
		w.cfg.Logger.Infof("database upgrade for %v already completed", w.toVersion)
		w.cfg.UpgradeCompleteLock.Unlock()
		return true
	}

	return false
}

func (w *upgradeDBWorker) runUpgrade() error {
	w.cfg.Logger.Infof("running database upgrade database")

	// This dummy worker, so we can just Unlock...
	w.cfg.Logger.Infof("database upgrade already completed")
	w.cfg.UpgradeCompleteLock.Unlock()

	return nil
}
