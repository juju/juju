// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5-unstable/hooks"

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
	rh.name = name
	rh.runner = rnr

	if err := rh.beforeHook(); err != nil {
		return nil, err
	}

	return stateChange{
		Kind: RunHook,
		Step: Pending,
		Hook: &rh.info,
	}.apply(state), nil
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
	return rh.callbacks.SetExecutingStatus(fmt.Sprintf("running %s hook", rh.info.Kind))
}

func (rh *runHook) afterHook(state State) (*State, error) {
	ctx := rh.runner.Context()
	hasRunStatusSet := ctx.HasRunSetUnitStatus()
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
		return nil, err
	}
	return stateChange{
		HasRunStatusSet: &hasRunStatusSet,
	}.apply(state), nil
}

// Execute runs the hook.
// Execute is part of the Operation interface.
func (rh *runHook) Execute(state State) (*State, error) {
	message := fmt.Sprintf("running hook %s", rh.name)
	unlock, err := rh.callbacks.AcquireExecutionLock(message)
	if err != nil {
		return nil, err
	}
	defer unlock()

	ranHook := true
	step := Done

	err = rh.runner.RunHook(rh.name)
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
		afterHookState, err := rh.afterHook(state)
		if err != nil {
			return nil, err
		}
		state = *afterHookState
	} else {
		logger.Infof("skipped %q hook (missing)", rh.name)
	}
	return stateChange{
		Kind: RunHook,
		Step: step,
		Hook: &rh.info,
	}.apply(state), err
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
	}

	return newState, nil
}
