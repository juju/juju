// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"time"

	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

type runHook struct {
	info hook.Info

	paths          context.Paths
	callbacks      Callbacks
	contextFactory context.Factory

	name    string
	context context.Context
}

func (rh *runHook) String() string {
	suffix := ""
	if rh.info.Kind.IsRelation() {
		if rh.info.RemoteUnit == "" {
			suffix = fmt.Sprintf(" (%d)", rh.info.RelationId)
		} else {
			suffix = fmt.Sprintf(" (%d; %s)", rh.info.RelationId, rh.info.RemoteUnit)
		}
	}
	return fmt.Sprintf("run %s%s hook", rh.info.Kind, suffix)
}

func (rh *runHook) Prepare(state State) (*State, error) {
	name, err := rh.callbacks.PrepareHook(rh.info)
	if err != nil {
		return nil, err
	}
	ctx, err := rh.contextFactory.NewHookContext(rh.info)
	if err != nil {
		return nil, err
	}
	rh.name = name
	rh.context = ctx
	return stateChange{
		Kind: RunHook,
		Step: Pending,
		Hook: &rh.info,
	}.apply(state), nil
}

func (rh *runHook) Execute(state State) (*State, error) {
	message := fmt.Sprintf("running hook %s", rh.name)
	unlock, err := rh.callbacks.AcquireExecutionLock(message)
	if err != nil {
		return nil, err
	}
	defer unlock()

	runner := context.NewRunner(rh.context, rh.paths)
	ranHook := true
	step := Done

	err = runner.RunHook(rh.name)
	switch {
	case context.IsMissingHookError(err):
		ranHook = false
		err = nil
	case err == context.ErrRequeueAndReboot:
		step = Queued
		fallthrough
	case err == context.ErrReboot:
		err = ErrNeedsReboot
	case err == nil:
	default:
		logger.Errorf("hook %q failed: %v", rh.name, err)
		rh.callbacks.NotifyHookFailed(rh.name, rh.context)
		return nil, ErrHookFailed
	}

	if ranHook {
		logger.Infof("ran %q hook", rh.name)
		rh.callbacks.NotifyHookCompleted(rh.name, rh.context)
	} else {
		logger.Infof("skipped %q hook (missing)", rh.name)
	}
	return stateChange{
		Kind: RunHook,
		Step: step,
		Hook: &rh.info,
	}.apply(state), err
}

func (rh *runHook) Commit(state State) (*State, error) {
	if err := rh.callbacks.CommitHook(rh.info); err != nil {
		return nil, err
	}
	newState := stateChange{
		Kind: Continue,
		Step: Pending,
		Hook: &rh.info,
	}.apply(state)
	switch rh.info.Kind {
	case hooks.Start:
		newState.Started = true
	case hooks.CollectMetrics:
		newState.CollectMetricsTime = time.Now().Unix()
	}
	return newState, nil
}
