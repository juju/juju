// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// NewMachineWorker returns a new instance of the machineWorker. It
// will run any required steps to upgrade a machine to the currently running
// Juju version.
func NewMachineWorker(
	upgradeCompleteLock gate.Lock,
	agent agent.Agent,
	apiCaller base.APICaller,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	performUpgradeSteps upgrades.UpgradeStepsFunc,
	logger Logger,
) worker.Worker {
	return newMachineWorker(&baseWorker{
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
	})
}

func newMachineWorker(base *baseWorker) *machineWorker {
	w := &machineWorker{
		baseWorker: base,
	}
	w.tomb.Go(w.run)
	return w
}

type machineWorker struct {
	*baseWorker

	tomb tomb.Tomb
}

// Kill is part of the worker.Worker interface.
func (w *machineWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *machineWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *machineWorker) run() error {
	// We're already upgraded, so do nothing.
	if w.alreadyUpgraded() {
		return nil
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	// Run the upgrade steps for a machine.
	if err := w.runUpgrades(ctx); err != nil {
		// Only return an error from the worker if the connection to
		// state went away (possible mongo primary change). Returning
		// an error when the connection is lost will cause the agent
		// to restart.
		if errors.Is(err, &apiLostDuringUpgrade{}) {
			return errors.Trace(err)
		}

		// We don't want to retry the upgrade if it failed, so signal
		// that the upgrade as being blocked.
		return nil
	}

	// Upgrade succeeded - signal that the upgrade is complete.
	w.logger.Infof("upgrade to %v completed successfully.", w.toVersion)
	_ = w.statusSetter.SetStatus(status.Started, "", nil)
	w.upgradeCompleteLock.Unlock()
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *machineWorker) runUpgrades(ctx context.Context) error {
	// Every upgrade needs to prepare the environment for the upgrade.
	w.logger.Infof("checking that upgrade can proceed")
	if err := w.preUpgradeSteps(w.agent.CurrentConfig(), false); err != nil {
		return errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.tag))
	}

	w.logger.Infof("running upgrade steps for %q", w.tag)
	if err := w.agent.ChangeConfig(w.runUpgradeSteps(ctx, []upgrades.Target{
		upgrades.HostMachine,
	})); err != nil {
		return errors.Annotatef(err, "failed to run upgrade steps")
	}

	return nil
}

func (w *machineWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}

type noopStatusSetter struct{}

func (noopStatusSetter) SetStatus(setableStatus status.Status, info string, data map[string]any) error {
	return nil
}
