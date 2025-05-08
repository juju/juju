// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/upgrade"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	"github.com/juju/juju/internal/worker/gate"
)

// UpgradeService is the interface for the upgrade service.
type UpgradeService interface {
	// SetControllerDone marks the supplied controllerID as having
	// completed its upgrades. When SetControllerDone is called by the
	// last provisioned controller, the upgrade will be archived.
	SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// SetDBUpgradeFailed marks the upgrade as failed in the database.
	// Manual intervention will be required if this has been invoked.
	SetDBUpgradeFailed(ctx context.Context, upgradeUUID domainupgrade.UUID) error
	// ActiveUpgrade returns the uuid of the current active upgrade. If there
	// are no active upgrades, return an upgradeerrors.NotFound error.
	ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error)
	// UpgradeInfo returns the upgrade info for the supplied upgradeUUID. If
	// there are no active upgrades, return an upgradeerrors.NotFound error.
	UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error)
	// WatchForUpgradeState creates a watcher which notifies when the upgrade
	// has reached the given state.
	WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error)
}

// NewControllerWorker returns a new instance of the controllerWorker worker. It
// will run any required steps to upgrade to the currently running
// Juju version.
func NewControllerWorker(
	upgradeCompleteLock gate.Lock,
	agent agent.Agent,
	apiCaller base.APICaller,
	upgradeService UpgradeService,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	performUpgradeSteps upgrades.UpgradeStepsFunc,
	entity upgradesteps.StatusSetter,
	logger logger.Logger,
	clock clock.Clock,
) (worker.Worker, error) {
	return newControllerWorker(
		&upgradesteps.BaseWorker{
			Agent:               agent,
			APICaller:           apiCaller,
			Tag:                 agent.CurrentConfig().Tag(),
			UpgradeCompleteLock: upgradeCompleteLock,
			PreUpgradeSteps:     preUpgradeSteps,
			PerformUpgradeSteps: performUpgradeSteps,
			StatusSetter:        entity,
			FromVersion:         agent.CurrentConfig().UpgradedToVersion(),
			ToVersion:           jujuversion.Current,
			Logger:              logger,
			Clock:               clock,
		},
		upgradeService,
	)
}

func newControllerWorker(base *upgradesteps.BaseWorker, upgradeService UpgradeService) (*controllerWorker, error) {
	w := &controllerWorker{
		base:           base,
		upgradeService: upgradeService,
		logger:         base.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "controller-upgrade",
		Site: &w.catacomb,
		Work: w.run,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type controllerWorker struct {
	base *upgradesteps.BaseWorker

	catacomb       catacomb.Catacomb
	upgradeService UpgradeService
	logger         logger.Logger
}

// Kill is part of the worker.Worker interface.
func (w *controllerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *controllerWorker) run() error {
	if w.base.AlreadyUpgraded() {
		return nil
	}

	// The pattern for the following controller worker is to watch for the
	// upgrade steps to be completed by all the controllers. Only when all
	// the controllers have completed their upgrade steps can we proceed.
	// If there is a failure, we'll abort the upgrade and wait for the user
	// to intervene.
	// The strategy for error reporting is to return the error if it's retryable
	// and to return nil if it is not. This is because then the worker will
	// be restarted and the upgrade will be retried. If the error is nil, it
	// will require user intervention to resolve the issue.

	ctx, cancel := w.scopedContext()
	defer cancel()

	// Locate the active upgrade. As the prior worker was the upgrade database
	// worker, this should have left us in a active upgrade state.
	upgradeUUID, err := w.upgradeService.ActiveUpgrade(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Verify the active upgrade information is at the correct state.
	info, err := w.upgradeService.UpgradeInfo(ctx, upgradeUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// We're not in the right state, so we can't proceed.
	if info.State != upgrade.DBCompleted {
		w.logger.Errorf(ctx, "upgrade %q is not in the db completed state %q", upgradeUUID, info.State.String())
		return w.abort(ctx, upgradeUUID, upgradesteps.ErrUpgradeStepsInvalidState)
	}

	// Watch for all the upgrade steps to be completed by all the controllers.
	// Only when all the controllers have completed their upgrade steps can
	// we proceed.
	completedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.StepsCompleted)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.addWatcher(ctx, completedWatcher); err != nil {
		return errors.Trace(err)
	}

	failedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.Error)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.addWatcher(ctx, failedWatcher); err != nil {
		return errors.Trace(err)
	}

	// Kick off the upgrade steps for the controller in a new managed context.
	stepsWorker := newControllerStepsWorker(w.base)
	if err := w.catacomb.Add(stepsWorker); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			w.logger.Errorf(ctx, "upgrade worker is dying whilst performing upgrade steps: %s, marking upgrade as failed", upgradeUUID)
			// We didn't perform the upgrade, so we need to mark it as failed.
			if err := w.upgradeService.SetDBUpgradeFailed(ctx, upgradeUUID); err != nil {
				w.logger.Errorf(ctx, "failed to set db upgrade failed: %v, manual intervention required.", err)
			}
			return w.catacomb.ErrDying()

		case <-completedWatcher.Changes():
			// All the controllers have completed their upgrade steps, so
			// we can now proceed with the upgrade.
			w.logger.Infof(ctx, "upgrade to %v completed successfully.", w.base.ToVersion)
			_ = w.base.StatusSetter.SetStatus(ctx, status.Started, "", nil)
			w.base.UpgradeCompleteLock.Unlock()

			return nil

		case <-failedWatcher.Changes():
			// One or all of the controllers have failed their upgrade steps,
			// so we can't proceed with the upgrade.
			w.logger.Errorf(ctx, "upgrade steps failed")
			return w.abort(ctx, upgradeUUID, upgradesteps.ErrFailedUpgradeSteps)

		case err := <-stepsWorker.Err():
			// The upgrade steps worker has completed, so we can now proceed
			// with the upgrade.
			if err != nil {
				// Only return an error from the worker if the connection was lost
				// whilst running upgrades. Returning an error when the connection is
				// lost will cause the agent to restart.
				if errors.Is(err, &upgradesteps.APILostDuringUpgrade{}) {
					return errors.Trace(err)
				}
				// If any of the steps have failed, abort the upgrade steps
				// and wait for the user to intervene.
				return w.abort(ctx, upgradeUUID, err)
			}

			// Mark the upgrade as completed for this controller machine.
			if err := w.upgradeService.SetControllerDone(ctx, upgradeUUID, w.base.Tag.Id()); err != nil {
				// We failed to mark the upgrade as completed, so we can't
				// proceed. We'll report the error and wait for the user to
				// intervene.
				return w.abort(ctx, upgradeUUID, err)
			}

			// We now wait for all the other controllers to complete before
			// we can proceed.
			continue

		case <-w.base.Clock.After(upgradesteps.DefaultUpgradeTimeout):
			// We've timed out waiting for the upgrade steps to complete.
			w.logger.Errorf(ctx, "timed out waiting for upgrade steps to complete")
			return w.abort(ctx, upgradeUUID, upgradesteps.ErrUpgradeTimeout)
		}
	}
}

func (w *controllerWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *controllerWorker) addWatcher(ctx context.Context, watcher eventsource.Watcher[struct{}]) error {
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

func (w *controllerWorker) abort(ctx context.Context, upgradeUUID domainupgrade.UUID, err error) error {
	// Set the status to error, we can't proceed with the upgrade.
	// Ignore the error as it's not critical if it fails.
	_ = w.base.StatusSetter.SetStatus(ctx, status.Error, "failed to perform upgrade steps, check logs.", nil)

	w.logger.Errorf(ctx, "aborting upgrade steps: %v, manual intervention is required", err)
	if err := w.upgradeService.SetDBUpgradeFailed(ctx, upgradeUUID); err != nil {
		w.logger.Errorf(ctx, "unable to fail upgrade steps %v.\nmanual intervention is required to force the upgrade state into an error state before proceeding", err)
	}
	return nil
}

// controllerStepsWorker is a worker that runs the upgrade steps for the
// controller. It is responsible for only running the upgrade steps and then
// reports the outcome to the Err method.
type controllerStepsWorker struct {
	base *upgradesteps.BaseWorker

	tomb tomb.Tomb

	status chan error
	logger logger.Logger
}

func newControllerStepsWorker(base *upgradesteps.BaseWorker) *controllerStepsWorker {
	w := &controllerStepsWorker{
		base:   base,
		status: make(chan error),
		logger: base.Logger,
	}
	w.tomb.Go(w.run)
	return w
}

// Kill is part of the worker.Worker interface.
func (w *controllerStepsWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerStepsWorker) Wait() error {
	return w.tomb.Wait()
}

// Err returns a channel that will report the err of the upgrade steps
// worker once it's completed.
func (w *controllerStepsWorker) Err() <-chan error {
	return w.status
}

func (w *controllerStepsWorker) run() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying

	case w.status <- w.runUpgrades(ctx):
		return nil
	}
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *controllerStepsWorker) runUpgrades(ctx context.Context) error {
	w.logger.Infof(ctx, "checking that upgrade can proceed")
	if err := w.base.PreUpgradeSteps(w.base.Agent.CurrentConfig(), false); err != nil {
		return errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.base.Tag))
	}

	w.logger.Infof(ctx, "running upgrade steps for %q", w.base.Tag)
	if err := w.base.Agent.ChangeConfig(w.base.RunUpgradeSteps(ctx, []upgrades.Target{
		upgrades.Controller,
		upgrades.HostMachine,
	})); err != nil {
		return errors.Annotatef(err, "failed to run upgrade steps")
	}

	return nil
}

func (w *controllerStepsWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
