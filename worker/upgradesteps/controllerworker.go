// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// UpgradeService is the interface for the upgrade service.
type UpgradeService interface {
	// SetControllerDone marks the supplied controllerID as having
	// completed its upgrades. When SetControllerDone is called by the
	// last provisioned controller, the upgrade will be archived.
	SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// ActiveUpgrade returns the uuid of the current active upgrade.
	// If there are no active upgrades, return a NotFound error
	ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error)
	// // UpgradeInfo returns the upgrade info for the supplied upgradeUUID.
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
	entity StatusSetter,
	logger Logger,
	clock clock.Clock,
) (worker.Worker, error) {
	return newControllerWorker(
		&baseWorker{
			agent:               agent,
			apiCaller:           apiCaller,
			tag:                 agent.CurrentConfig().Tag(),
			upgradeCompleteLock: upgradeCompleteLock,
			preUpgradeSteps:     preUpgradeSteps,
			performUpgradeSteps: performUpgradeSteps,
			statusSetter:        noopStatusSetter{},
			fromVersion:         agent.CurrentConfig().UpgradedToVersion(),
			toVersion:           jujuversion.Current,
			logger:              logger,
			clock:               clock,
		},
		upgradeService,
	)
}

func newControllerWorker(base *baseWorker, upgradeService UpgradeService) (*controllerWorker, error) {
	w := &controllerWorker{
		baseWorker:     base,
		upgradeService: upgradeService,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.run,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type controllerWorker struct {
	*baseWorker

	catacomb       catacomb.Catacomb
	upgradeService UpgradeService
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
	if w.alreadyUpgraded() {
		return nil
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	// Locate the active upgrade. As the prior worker was the upgrade database
	// worker, this should have left us in a active upgrade state.
	upgradeUUID, err := w.upgradeService.ActiveUpgrade(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Verify the the active upgrade information is at the correct state.
	info, err := w.upgradeService.UpgradeInfo(ctx, upgradeUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// We're not in the right state, so we can't proceed.
	if info.State != upgrade.DBCompleted {
		w.logger.Errorf("upgrade %q is not in the db completed state %q", upgradeUUID, info.State.String())
		return ErrUpgradeStepsInvalidState
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
	stepsWorker := newControllerStepsWorker(w.baseWorker)
	if err := w.catacomb.Add(stepsWorker); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-completedWatcher.Changes():
			// All the controllers have completed their upgrade steps, so
			// we can now proceed with the upgrade.
			w.logger.Infof("upgrade to %v completed successfully.", w.toVersion)
			_ = w.statusSetter.SetStatus(status.Started, "", nil)
			w.upgradeCompleteLock.Unlock()

			return nil

		case <-failedWatcher.Changes():
			// One or all of the controllers have failed their upgrade steps,
			// so we can't proceed with the upgrade.
			return w.abort(ErrFailedUpgradeSteps)

		case err := <-stepsWorker.Err():
			// The upgrade steps worker has completed, so we can now proceed
			// with the upgrade.
			if err != nil {
				// Only return an error from the worker if the connection was lost
				// whilst running upgrades. Returning an error when the connection is
				// lost will cause the agent to restart.
				if errors.Is(err, &apiLostDuringUpgrade{}) {
					return errors.Trace(err)
				}
				return nil
			}

			// Mark the upgrade as completed for this controller machine.
			if err := w.upgradeService.SetControllerDone(ctx, upgradeUUID, w.tag.Id()); err != nil {
				// We failed to mark the upgrade as completed, so we can't
				// proceed. We'll report the error and wait for the user to
				// intervene.
				w.reportUpgradeFailure(err, true)
				return nil
			}

			// We now wait for all the other controllers to complete before
			// we can proceed.
			continue

		case <-w.clock.After(defaultUpgradeTimeout):
			// We've timed out waiting for the upgrade steps to complete.
			return w.abort(ErrUpgradeTimeout)
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

func (w *controllerWorker) abort(err error) error {
	// TODO (stickupkid): Set the failed error state.
	w.logger.Errorf("aborting upgrade steps: %v", err)
	return err
}

// controllerStepsWorker is a worker that runs the upgrade steps for the
// controller. It is responsible for only running the upgrade steps and then
// reports the outcome to the Err method.
type controllerStepsWorker struct {
	*baseWorker

	tomb tomb.Tomb

	status chan error
}

func newControllerStepsWorker(base *baseWorker) *controllerStepsWorker {
	w := &controllerStepsWorker{
		baseWorker: base,
		status:     make(chan error),
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
	w.logger.Infof("checking that upgrade can proceed")
	if err := w.preUpgradeSteps(w.agent.CurrentConfig(), false); err != nil {
		return errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.tag))
	}

	w.logger.Infof("running upgrade steps for %q", w.tag)
	if err := w.agent.ChangeConfig(w.runUpgradeSteps(ctx, []upgrades.Target{
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
