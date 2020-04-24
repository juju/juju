// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/runner/jujuc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type runHook struct {
	info hook.Info

	callbacks     Callbacks
	runnerFactory runner.Factory

	name   string
	runner runner.Runner
	logger Logger

	hookFound bool

	RequiresMachineLock
}

// String is part of the Operation interface.
func (rh *runHook) String() string {
	suffix := ""
	switch {
	case rh.info.Kind.IsRelation():
		if rh.info.RemoteUnit == "" {
			suffix = fmt.Sprintf(" (%d; app: %s)", rh.info.RelationId, rh.info.RemoteApplication)
		} else if rh.info.DepartingUnit != "" {
			suffix = fmt.Sprintf(" (%d; unit: %s, departee: %s)", rh.info.RelationId, rh.info.RemoteUnit, rh.info.DepartingUnit)
		} else {
			suffix = fmt.Sprintf(" (%d; unit: %s)", rh.info.RelationId, rh.info.RemoteUnit)
		}
	case rh.info.Kind.IsStorage():
		suffix = fmt.Sprintf(" (%s)", rh.info.StorageId)
	}
	return fmt.Sprintf("run %s%s hook", rh.info.Kind, suffix)
}

// Prepare ensures the hook can be executed.
// Prepare is part of the Operation interface.
func (rh *runHook) Prepare(state State) (*State, error) {
	name, err := rh.callbacks.PrepareHook(rh.info)
	if err != nil {
		return nil, err
	}
	rnr, err := rh.runnerFactory.NewHookRunner(rh.info)
	if err != nil {
		return nil, err
	}

	if hooks.Kind(name) == hooks.LeaderElected {
		// Check if leadership has changed between queueing of the hook and
		// Actual execution. Skip execution if we are no longer the leader.
		var isLeader bool
		isLeader, err = rnr.Context().IsLeader()
		if err == nil && !isLeader {
			rh.logger.Infof("unit is no longer the leader; skipping %q execution", name)
			return nil, ErrSkipExecute
		}
		if err != nil {
			return nil, err
		}
	}

	err = rnr.Context().Prepare()
	if err != nil {
		return nil, errors.Trace(err)
	}
	rh.name = name
	rh.runner = rnr

	return stateChange{
		Kind: RunHook,
		Step: Pending,
		Hook: &rh.info,
	}.apply(state), nil
}

// RunningHookMessage returns the info message to print when running a hook.
func RunningHookMessage(hookName string) string {
	return fmt.Sprintf("running %s hook", hookName)
}

// Execute runs the hook.
// Execute is part of the Operation interface.
func (rh *runHook) Execute(state State) (*State, error) {
	message := RunningHookMessage(rh.name)
	if err := rh.beforeHook(state); err != nil {
		return nil, err
	}
	// In order to reduce controller load, the uniter no longer
	// records when it is running the update-status hook. If the
	// hook fails, that is recorded.
	if hooks.Kind(rh.name) != hooks.UpdateStatus {
		if err := rh.callbacks.SetExecutingStatus(message); err != nil {
			return nil, err
		}
	}
	// The before hook may have updated unit status and we don't want that
	// to count so reset it here before running the hook.
	rh.runner.Context().ResetExecutionSetUnitStatus()

	rh.hookFound = true
	step := Done

	handlerType, err := rh.runner.RunHook(rh.name)
	cause := errors.Cause(err)
	switch {
	case charmrunner.IsMissingHookError(cause):
		rh.hookFound = false
		err = nil
	case cause == context.ErrRequeueAndReboot:
		step = Queued
		fallthrough
	case cause == context.ErrReboot:
		err = ErrNeedsReboot
	case err == nil:
	default:
		rh.logger.Errorf("hook %q (via %s) failed: %v", rh.name, handlerType, err)
		rh.callbacks.NotifyHookFailed(rh.name, rh.runner.Context())
		return nil, ErrHookFailed
	}

	if rh.hookFound {
		rh.logger.Infof("ran %q hook (via %s)", rh.name, handlerType)
		rh.callbacks.NotifyHookCompleted(rh.name, rh.runner.Context())
	} else {
		rh.logger.Infof("skipped %q hook (missing)", rh.name)
	}

	var hasRunStatusSet bool
	var afterHookErr error
	if hasRunStatusSet, afterHookErr = rh.afterHook(state); afterHookErr != nil {
		return nil, afterHookErr
	}
	return stateChange{
		Kind:            RunHook,
		Step:            step,
		Hook:            &rh.info,
		HasRunStatusSet: hasRunStatusSet,
	}.apply(state), err
}

func (rh *runHook) beforeHook(state State) error {
	var err error
	switch rh.info.Kind {
	case hooks.Install:
		// If the charm has already updated the unit status in a previous hook,
		// then don't overwrite that here.
		if !state.StatusSet {
			err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
				Status: string(status.Maintenance),
				Info:   status.MessageInstallingCharm,
			})
		}
	case hooks.Stop:
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(status.Maintenance),
			Info:   "stopping charm software",
		})
	case hooks.Remove:
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(status.Maintenance),
			Info:   "cleaning up prior to charm deletion",
		})
	case hooks.PreSeriesUpgrade:
		err = rh.callbacks.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareRunning, "pre-series-upgrade hook running")
	case hooks.PostSeriesUpgrade:
		err = rh.callbacks.SetUpgradeSeriesStatus(model.UpgradeSeriesCompleteRunning, "post-series-upgrade hook running")
	}

	if err != nil {
		rh.logger.Errorf("error updating workload status before %v hook: %v", rh.info.Kind, err)
		return err
	}
	return nil
}

// afterHook runs after a hook completes, or after a hook that is
// not implemented by the charm is expected to have run if it were
// implemented.
func (rh *runHook) afterHook(state State) (_ bool, err error) {
	defer func() {
		if err != nil {
			rh.logger.Errorf("error updating workload status after %v hook: %v", rh.info.Kind, err)
		}
	}()

	ctx := rh.runner.Context()
	hasRunStatusSet := ctx.HasExecutionSetUnitStatus() || state.StatusSet
	switch rh.info.Kind {
	case hooks.Stop:
		err = ctx.SetUnitStatus(jujuc.StatusInfo{
			Status: string(status.Maintenance),
		})
	case hooks.Remove:
		// Charm is no longer of this world.
		err = ctx.SetUnitStatus(jujuc.StatusInfo{
			Status: string(status.Terminated),
		})
	case hooks.Start:
		if hasRunStatusSet {
			break
		}
		rh.logger.Debugf("unit %v has started but has not yet set status", ctx.UnitName())
		// We've finished the start hook and the charm has not updated its
		// own status so we'll set it to unknown.
		err = ctx.SetUnitStatus(jujuc.StatusInfo{
			Status: string(status.Unknown),
		})
	case hooks.RelationBroken:
		var isLeader bool
		isLeader, err = ctx.IsLeader()
		if !isLeader || err != nil {
			return hasRunStatusSet && err == nil, err
		}
		rel, rErr := ctx.Relation(rh.info.RelationId)
		if rErr == nil && rel.Suspended() {
			err = rel.SetStatus(relation.Suspended)
		}
	}
	return hasRunStatusSet && err == nil, err
}

func createUpgradeSeriesStatusMessage(name string, hookFound bool) string {
	if !hookFound {
		return fmt.Sprintf("%s hook not found, skipping", name)
	}
	return fmt.Sprintf("%s completed", name)
}

// Commit updates relation state to include the fact of the hook's execution,
// records the impact of start and collect-metrics hooks, and queues follow-up
// config-changed hooks to directly follow install and upgrade-charm hooks.
// Commit is part of the Operation interface.
func (rh *runHook) Commit(state State) (*State, error) {
	var err error
	err = rh.callbacks.CommitHook(rh.info)
	if err != nil {
		return nil, err
	}

	change := stateChange{
		Kind: Continue,
		Step: Pending,
	}

	switch rh.info.Kind {
	case hooks.ConfigChanged:
		if !state.Started {
			change = stateChange{
				Kind: RunHook,
				Step: Queued,
				Hook: &hook.Info{Kind: hooks.Start},
			}
		}
	case hooks.UpgradeCharm:
		change = stateChange{
			Kind: RunHook,
			Step: Queued,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}
	case hooks.PreSeriesUpgrade:
		message := createUpgradeSeriesStatusMessage(rh.name, rh.hookFound)
		err = rh.callbacks.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, message)
	case hooks.PostSeriesUpgrade:
		message := createUpgradeSeriesStatusMessage(rh.name, rh.hookFound)
		err = rh.callbacks.SetUpgradeSeriesStatus(model.UpgradeSeriesCompleted, message)
	}
	if err != nil {
		return nil, err
	}

	newState := change.apply(state)

	switch rh.info.Kind {
	case hooks.Install:
		newState.Installed = true
	case hooks.Start:
		newState.Started = true
	case hooks.Stop:
		newState.Stopped = true
	case hooks.Remove:
		newState.Removed = true
	}

	return newState, nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (rh *runHook) RemoteStateChanged(snapshot remotestate.Snapshot) {
}
