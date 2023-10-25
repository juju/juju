// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsmachine

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/upgradesteps"
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
	statusSetter upgradesteps.StatusSetter,
	logger upgradesteps.Logger,
	clock clock.Clock,
) worker.Worker {
	return newMachineWorker(&upgradesteps.BaseWorker{
		Agent:               agent,
		APICaller:           apiCaller,
		Tag:                 agent.CurrentConfig().Tag(),
		UpgradeCompleteLock: upgradeCompleteLock,
		PreUpgradeSteps:     preUpgradeSteps,
		PerformUpgradeSteps: performUpgradeSteps,
		StatusSetter:        statusSetter,
		FromVersion:         agent.CurrentConfig().UpgradedToVersion(),
		ToVersion:           jujuversion.Current,
		Logger:              logger,
		Clock:               clock,
	})
}

func newMachineWorker(base *upgradesteps.BaseWorker) *machineWorker {
	w := &machineWorker{
		base:   base,
		logger: base.Logger,
	}
	w.tomb.Go(w.run)
	return w
}

type machineWorker struct {
	base *upgradesteps.BaseWorker

	tomb tomb.Tomb

	logger Logger
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
	if w.base.AlreadyUpgraded() {
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
		if errors.Is(err, &upgradesteps.APILostDuringUpgrade{}) {
			return errors.Trace(err)
		}

		// We don't want to retry the upgrade if it failed, so signal
		// that the upgrade as being blocked.
		return nil
	}

	// Upgrade succeeded - signal that the upgrade is complete.
	w.logger.Infof("upgrade to %v completed successfully.", w.base.ToVersion)
	_ = w.base.StatusSetter.SetStatus(status.Started, "", nil)
	w.base.UpgradeCompleteLock.Unlock()
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *machineWorker) runUpgrades(ctx context.Context) error {
	// Every upgrade needs to prepare the environment for the upgrade.
	w.logger.Infof("checking that upgrade can proceed")
	if err := w.base.PreUpgradeSteps(w.base.Agent.CurrentConfig(), false); err != nil {
		return errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.base.Tag))
	}

	w.logger.Infof("running upgrade steps for %q", w.base.Tag)
	if err := w.base.Agent.ChangeConfig(w.base.RunUpgradeSteps(ctx, []upgrades.Target{
		upgrades.HostMachine,
	})); err != nil {
		return errors.Annotatef(err, "failed to run upgrade steps")
	}

	return nil
}

func (w *machineWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
