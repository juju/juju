// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/schema"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/worker/gate"
)

const (
	// defaultUpgradeTimeout is the default timeout for the upgrade to complete.
	// 20 minutes should be enough for the db upgrade to complete.
	defaultUpgradeTimeout = 20 * time.Minute
)

// UpgradeService is the interface for the upgrade service.
type UpgradeService interface {
	// CreateUpgrade creates an upgrade to and from specified versions
	// If an upgrade is already running/pending, return an AlreadyExists err
	CreateUpgrade(ctx context.Context, previousVersion, targetVersion semversion.Number) (domainupgrade.UUID, error)
	// SetControllerReady marks the supplied controllerID as being ready
	// to start its upgrade. All provisioned controllers need to be ready
	// before an upgrade can start
	SetControllerReady(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// StartUpgrade starts the current upgrade if it exists. If it is already
	// started it will return an AlreadyStarted error.
	StartUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// SetDBUpgradeCompleted marks the upgrade as completed in the database
	SetDBUpgradeCompleted(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// SetDBUpgradeFailed marks the upgrade as failed in the database
	SetDBUpgradeFailed(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// ActiveUpgrade returns the uuid of the current active upgrade. If there
	// are no active upgrades, return an upgradeerrors.NotFound error.
	ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error)
	// UpgradeInfo returns the upgrade info for the supplied upgradeUUID. If
	// there are no active upgrades, return an upgradeerrors.NotFound error.
	UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error)
	// WatchForUpgradeReady creates a watcher which notifies when all controller
	// nodes have been registered, meaning the upgrade is ready to start.
	WatchForUpgradeReady(ctx context.Context, upgradeUUID domainupgrade.UUID) (watcher.NotifyWatcher, error)
	// WatchForUpgradeState creates a watcher which notifies when the upgrade
	// has reached the given state.
	WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error)
}

// ModelService is the interface for the model service.
type ModelService interface {
	// ListModelUUIDs returns a list of all model UUIDs that are active in the
	// controller.
	ListModelUUIDs(context.Context) ([]coremodel.UUID, error)
}

// Config holds the configuration for the worker.
type Config struct {
	// DBUpgradeCompleteLock is a lock used to synchronise workers that must
	// start after database upgrades are verified as completed.
	DBUpgradeCompleteLock gate.Lock

	// Agent is the running machine agent.
	Agent agent.Agent

	// ModelService is the model manager service used to identify
	// the model uuids required to upgrade.
	ModelService ModelService

	// UpgradeService is the upgrade service used to drive the upgrade.
	UpgradeService UpgradeService

	// DBGetter is the database getter used to get the database for each model.
	DBGetter coredatabase.DBGetter

	// Tag holds the controller tag information.
	Tag names.Tag

	// Versions of the source and destination.
	FromVersion semversion.Number
	ToVersion   semversion.Number

	Logger logger.Logger
	Clock  clock.Clock
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
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.FromVersion == semversion.Zero {
		return errors.NotValidf("invalid FromVersion")
	}
	if c.ToVersion == semversion.Zero {
		return errors.NotValidf("invalid ToVersion")
	}
	if c.Tag == nil {
		return errors.NotValidf("invalid Tag")
	}
	return nil
}

type upgradeDBWorker struct {
	catacomb catacomb.Catacomb

	dbUpgradeCompleteLock gate.Lock

	controllerID string

	fromVersion semversion.Number
	toVersion   semversion.Number

	dbGetter coredatabase.DBGetter

	modelService   ModelService
	upgradeService UpgradeService

	logger logger.Logger
	clock  clock.Clock
}

// NewUpgradeDatabaseWorker returns a new Worker.
func NewUpgradeDatabaseWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeDBWorker{
		dbUpgradeCompleteLock: config.DBUpgradeCompleteLock,

		controllerID: config.Tag.Id(),

		fromVersion: config.FromVersion,
		toVersion:   config.ToVersion,

		dbGetter: config.DBGetter,

		modelService:   config.ModelService,
		upgradeService: config.UpgradeService,

		logger: config.Logger,
		clock:  config.Clock,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "upgrade-database",
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	if w.upgradeDone(ctx) {
		// We're already upgraded, so we can uninstall this worker. This will
		// prevent it from running again, without an agent restart.
		return dependency.ErrUninstall
	}

	w.logger.Debugf(ctx, "attempting to create upgrade from: %v to: %v", w.fromVersion, w.toVersion)

	// Create an upgrade for this controller. If another controller has already
	// created the upgrade, we will get an AlreadyExists error. The job of this
	// controller is just to wait for the upgrade to be done and then unlock the
	// DBUpgradeCompleteLock.
	//
	// If the upgrade failed the previous time, we'll be allowed to create a
	// new upgrade. We don't want to block this, as this will brick all
	// controllers attempting upgrade and fail with an error.
	upgradeUUID, err := w.upgradeService.CreateUpgrade(ctx, w.fromVersion, w.toVersion)
	if err != nil {
		if errors.Is(err, upgradeerrors.AlreadyExists) {
			// We're already running the upgrade, so we can just watch the
			// upgrade and wait for it to complete.
			w.logger.Tracef(ctx, "upgrade already started, watching upgrade")
			return w.watchUpgrade(ctx)
		}
		w.logger.Errorf(ctx, "failed to create upgrade: %v\nmanual manual intervention is required", err)
		// Failed to set the upgrade as failed, we can't do anything
		// here. It requires a manual intervention to fix the problem.
		return nil
	}

	return w.runUpgrade(ctx, upgradeUUID)
}

// watchUpgrade watches the upgrade until it is complete.
// Once the upgrade is complete, the DBUpgradeCompleteLock is unlocked.
func (w *upgradeDBWorker) watchUpgrade(ctx context.Context) error {
	w.logger.Infof(ctx, "watching upgrade from: %v to: %v", w.fromVersion, w.toVersion)

	upgradeUUID, err := w.upgradeService.ActiveUpgrade(ctx)
	if err != nil {
		if errors.Is(err, upgradeerrors.NotFound) {
			// This currently no active upgrade, so we can't watch anything.
			// If this happens, it's probably in a bad state. We can't really
			// do anything about it, so we'll just bounce and hope that we
			// see if we've performed the upgrade already and that
			// we just didn't know about it in time.
			return dependency.ErrBounce
		}
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	info, err := w.upgradeService.UpgradeInfo(ctx, upgradeUUID)
	if err != nil {
		if errors.Is(err, upgradeerrors.NotFound) {
			// This currently no active upgrade, so we can't watch anything.
			// If this happens, it's probably in a bad state. We can't really
			// do anything about it, so we'll just bounce and hope that we
			// see if we've performed the upgrade already and that
			// we just didn't know about it in time.
			return dependency.ErrBounce
		}
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	if info.State == upgrade.Error {
		// We're in an error state, so we can't do anything about it, so we'll
		// make a note and kill the worker. It's then up to the user to fix the
		// problem and restart the agent.
		w.logger.Errorf(ctx, "database upgrade failed, already in an error state, check logs for details")
		return nil
	}

	completedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.DBCompleted)
	if err != nil {
		return w.abortWithError(ctx, upgradeUUID, errors.Annotate(err, "watch completed upgrade"))
	}

	if err := w.addWatcher(ctx, completedWatcher); err != nil {
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	failedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.Error)
	if err != nil {
		return w.abortWithError(ctx, upgradeUUID, errors.Annotate(err, "watch failed upgrade"))
	}

	if err := w.addWatcher(ctx, failedWatcher); err != nil {
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	// Mark this controller as ready to start the upgrade. We do this after
	// we've added the watchers, so that we don't miss any events. If this
	// fails, the agent will restart and try again.
	if err := w.upgradeService.SetControllerReady(ctx, upgradeUUID, w.controllerID); err != nil {
		// If the set controller ready fails, we'll abort the upgrade. This will
		// cause the upgrade to be marked as failed, and the next time the agent
		// restarts, it will try again.
		w.logger.Errorf(ctx, "failed to set controller ready: %v", err)
		return w.abort(ctx, upgradeUUID)
	}
	w.logger.Infof(ctx, "marking the controller ready for upgrade")

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-completedWatcher.Changes():
			// The upgrade is complete, so we can unlock the lock.
			w.logger.Infof(ctx, "database upgrade completed")
			w.dbUpgradeCompleteLock.Unlock()
			return dependency.ErrUninstall

		case <-failedWatcher.Changes():
			// If the upgrade failed, we can't do anything about it, we'll make
			// a note about the failure to upgrade. We'll return
			// dependency.ErrBounce, this will allow the workers to restart
			// and try again.
			w.logger.Errorf(ctx, "database upgrade failed, check logs for details")
			return dependency.ErrBounce
		}
	}
}

// upgradeDone returns true if this worker does not need to run any upgrade
// logic.
func (w *upgradeDBWorker) upgradeDone(ctx context.Context) bool {
	// If we are already unlocked, there is nothing to do.
	if w.dbUpgradeCompleteLock.IsUnlocked() {
		return true
	}

	if w.fromVersion == w.toVersion {
		w.logger.Infof(ctx, "database upgrade for %v already completed", w.toVersion)
		w.dbUpgradeCompleteLock.Unlock()
		return true
	}

	return false
}

func (w *upgradeDBWorker) runUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	w.logger.Infof(ctx, "leading the database upgrade from: %v to: %v", w.fromVersion, w.toVersion)

	ctx, cancel := w.scopedContext()
	defer cancel()

	// Watch for the upgrade to be ready. This should ensure that all
	// controllers are sync'd and waiting for the leader to start the upgrade.
	watcher, err := w.upgradeService.WatchForUpgradeReady(ctx, upgradeUUID)
	if err != nil {
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	if err := w.addWatcher(ctx, watcher); err != nil {
		return w.abortWithError(ctx, upgradeUUID, err)
	}

	// Ensure we mark this controller as ready to start the upgrade. We do this
	// after we've added the watcher, so that we don't miss any events.
	if err := w.upgradeService.SetControllerReady(ctx, upgradeUUID, w.controllerID); err != nil {
		// If the set controller ready fails, we'll abort the upgrade. This will
		// cause the upgrade to be marked as failed, and the next time the agent
		// restarts, it will try again.
		w.logger.Errorf(ctx, "failed to set controller ready: %v", err)
		return w.abortWithError(ctx, upgradeUUID, err)
	}
	w.logger.Infof(ctx, "marking the controller ready for upgrade")

	for {
		select {
		case <-w.catacomb.Dying():
			w.logger.Errorf(ctx, "upgrade worker is dying whilst performing upgrade: %s, marking upgrade as failed", upgradeUUID)
			// We didn't perform the upgrade, so we need to mark it as failed.
			if err := w.upgradeService.SetDBUpgradeFailed(ctx, upgradeUUID); err != nil {
				w.logger.Errorf(ctx, "failed to set db upgrade failed: %v, manual intervention required.", err)
			}
			return w.catacomb.ErrDying()

		case <-w.clock.After(defaultUpgradeTimeout):
			return w.abort(ctx, upgradeUUID)

		case <-watcher.Changes():
			w.logger.Infof(ctx, "database upgrade starting")

			// Any errors within this block will need to set the upgrade as
			// failed. Otherwise once the agent restarts upon the error, the
			// create upgrade will error out with AlreadyExists. This
			// will cause the controller to fall into the watching state. No
			// other controller will be able to start the upgrade, at they're
			// also in the watching state. No forward progress will be made.

			err := w.performUpgrade(ctx, upgradeUUID)
			if err == nil {
				w.logger.Infof(ctx, "database upgrade completed")
				w.dbUpgradeCompleteLock.Unlock()
				return dependency.ErrUninstall
			}

			w.logger.Errorf(ctx, "database upgrade failed, check logs for details")

			return w.abort(ctx, upgradeUUID)
		}
	}
}

func (w *upgradeDBWorker) abort(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	return w.abortWithError(ctx, upgradeUUID, dependency.ErrBounce)
}

// abort marks the upgrade as failed and returns dependency.ErrBounce.
func (w *upgradeDBWorker) abortWithError(ctx context.Context, upgradeUUID domainupgrade.UUID, err error) error {
	// Set the upgrade as failed, so that the next time the agent
	// restarts, it will try again.
	if err := w.upgradeService.SetDBUpgradeFailed(ctx, upgradeUUID); err != nil {
		w.logger.Errorf(ctx, "failed to set db upgrade failed: %v", err)

		// Failed to set the upgrade as failed, we can't do anything
		// here. It requires a manual intervention to fix the problem.
		return nil
	}

	return errors.Trace(err)
}

func (w *upgradeDBWorker) performUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	if err := w.upgradeService.StartUpgrade(ctx, upgradeUUID); err != nil {
		return errors.Annotatef(err, "start upgrade")
	}

	// Upgrade the controller database first.
	if err := w.upgradeController(ctx); err != nil {
		return errors.Trace(err)
	}
	// Then upgrade the models databases.
	if err := w.upgradeModels(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := w.upgradeService.SetDBUpgradeCompleted(ctx, upgradeUUID); err != nil {
		return errors.Annotatef(err, "set db upgrade completed")
	}

	return nil
}

func (w *upgradeDBWorker) upgradeController(ctx context.Context) error {
	w.logger.Infof(ctx, "upgrading controller database from: %v to: %v", w.fromVersion, w.toVersion)

	db, err := w.dbGetter.GetDB(ctx, coredatabase.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "controller db")
	}

	ddl := schema.ControllerDDL()
	changeSet, err := ddl.Ensure(ctx, db)
	if err != nil {
		return errors.Annotatef(err, "applying controller schema")
	}
	w.logger.Infof(ctx, "applied controller schema changes from: %d to: %d", changeSet.Post, changeSet.Current)
	return nil
}

func (w *upgradeDBWorker) upgradeModels(ctx context.Context) error {
	w.logger.Infof(ctx, "upgrading model databases from: %v to: %v", w.fromVersion, w.toVersion)

	models, err := w.modelService.ListModelUUIDs(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting model list")
	}

	for _, modelUUID := range models {
		if err := w.upgradeModel(ctx, modelUUID); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (w *upgradeDBWorker) upgradeModel(ctx context.Context, modelUUID coremodel.UUID) error {
	db, err := w.dbGetter.GetDB(ctx, modelUUID.String())
	if err != nil {
		return errors.Annotatef(err, "model db %s", modelUUID)
	}

	ddl := schema.ModelDDL()
	changeSet, err := ddl.Ensure(ctx, db)
	if err != nil {
		return errors.Annotatef(err, "applying model schema %s", modelUUID)
	}
	w.logger.Infof(ctx, "applied model schema changes from: %d to: %d for model %s", changeSet.Post, changeSet.Current, modelUUID)
	return nil
}

func (w *upgradeDBWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *upgradeDBWorker) addWatcher(ctx context.Context, watcher eventsource.Watcher[struct{}]) error {
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The notify watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[struct{}](ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}
