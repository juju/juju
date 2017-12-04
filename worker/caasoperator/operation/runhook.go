// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE store for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/common/charmrunner"
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
	if hooks.Kind(rh.name) != hooks.UpdateStatus {
		if err := rh.callbacks.SetExecutingStatus(message); err != nil {
			return nil, err
		}
	}

	ranHook := true
	err := rh.runner.RunHook(rh.name)
	cause := errors.Cause(err)
	switch {
	case charmrunner.IsMissingHookError(cause):
		ranHook = false
		err = nil
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

	return stateChange{
		Kind: RunHook,
		Step: Done,
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
	newState := change.apply(state)
	return newState, nil
}
