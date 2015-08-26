// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type runHook struct {
	info hook.Info

	callbacks     Callbacks
	runnerFactory runner.Factory

	name   string
	runner runner.Runner

	RequiresMachineLock
}

// String is part of the Operation interface.
func (rh *runHook) String() string {
	suffix := ""
	switch {
	case rh.info.Kind.IsRelation():
		if rh.info.RemoteUnit == "" {
			suffix = fmt.Sprintf(" (%d)", rh.info.RelationId)
		} else {
			suffix = fmt.Sprintf(" (%d; %s)", rh.info.RelationId, rh.info.RemoteUnit)
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
	if err := rh.beforeHook(); err != nil {
		return nil, err
	}
	if err := rh.callbacks.SetExecutingStatus(message); err != nil {
		return nil, err
	}
	// The before hook may have updated unit status and we don't want that
	// to count so reset it here before running the hook.
	rh.runner.Context().ResetExecutionSetUnitStatus()

	ranHook := true
	step := Done

	err := rh.runner.RunHook(rh.name)
	cause := errors.Cause(err)
	switch {
	case runner.IsMissingHookError(cause):
		ranHook = false
		err = nil
	case cause == runner.ErrRequeueAndReboot:
		step = Queued
		fallthrough
	case cause == runner.ErrReboot:
		err = ErrNeedsReboot
	case err == nil:
	default:
		logger.Errorf("hook %q failed: %v", rh.name, err)
		rh.callbacks.NotifyHookFailed(rh.name, rh.runner.Context())
		return nil, ErrHookFailed
	}

	if ranHook {
		logger.Infof("ran %q hook", rh.name)
		rh.callbacks.NotifyHookCompleted(rh.name, rh.runner.Context())
	} else {
		logger.Infof("skipped %q hook (missing)", rh.name)
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

func (rh *runHook) beforeHook() error {
	var err error
	switch rh.info.Kind {
	case hooks.Install:
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(params.StatusMaintenance),
			Info:   "installing charm software",
		})
	case hooks.Stop:
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(params.StatusMaintenance),
			Info:   "cleaning up prior to charm deletion",
		})
	}
	if err != nil {
		logger.Errorf("error updating workload status before %v hook: %v", rh.info.Kind, err)
		return err
	}
	return nil
}

// afterHook runs after a hook completes, or after a hook that is
// not implemented by the charm is expected to have run if it were
// implemented.
func (rh *runHook) afterHook(state State) (bool, error) {
	ctx := rh.runner.Context()
	hasRunStatusSet := ctx.HasExecutionSetUnitStatus() || state.StatusSet
	var err error
	switch rh.info.Kind {
	case hooks.Stop:
		// Charm is no longer of this world.
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(params.StatusTerminated),
		})
	case hooks.Start:
		if hasRunStatusSet {
			break
		}
		// We've finished the start hook and the charm has not updated its
		// own status so we'll set it to unknown.
		err = rh.runner.Context().SetUnitStatus(jujuc.StatusInfo{
			Status: string(params.StatusUnknown),
		})
	}
	if err != nil {
		logger.Errorf("error updating workload status after %v hook: %v", rh.info.Kind, err)
		return false, err
	}
	return hasRunStatusSet, nil
}

// Commit updates relation state to include the fact of the hook's execution,
// records the impact of start and collect-metrics hooks, and queues follow-up
// config-changed hooks to directly follow install and upgrade-charm hooks.
// Commit is part of the Operation interface.
func (rh *runHook) Commit(state State) (*State, error) {
	if err := rh.callbacks.CommitHook(rh.info); err != nil {
		return nil, err
	}

	change := stateChange{
		Kind: Continue,
		Step: Pending,
	}

	var hi *hook.Info = &hook.Info{Kind: hooks.ConfigChanged}
	switch rh.info.Kind {
	case hooks.ConfigChanged:
		if state.Started {
			break
		}
		hi.Kind = hooks.Start
		fallthrough
	case hooks.UpgradeCharm:
		change = stateChange{
			Kind: RunHook,
			Step: Queued,
			Hook: hi,
		}
	}

	newState := change.apply(state)

	switch rh.info.Kind {
	case hooks.Start:
		newState.Started = true
	case hooks.Stop:
		newState.Stopped = true
	case hooks.CollectMetrics:
		newState.CollectMetricsTime = time.Now().Unix()
	case hooks.UpdateStatus:
		newState.UpdateStatusTime = time.Now().Unix()
	}

	return newState, nil
}
