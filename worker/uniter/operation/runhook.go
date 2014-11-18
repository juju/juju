// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

type RunHookHelper interface {
	// PrepareHook and CommitHook exist so that I can defer worrying about how
	// to untangle Uniter.relationers from everything else.
	PrepareHook(info hook.Info) (name string, err error)
	CommitHook(info hook.Info) error

	// NotifyHook* exist so that I can defer worrying about how to untangle the
	// callbacks inserted for uniter_test.
	NotifyHookCompleted(string, context.Context)
	NotifyHookFailed(string, context.Context)
}

type runHook struct {
	info   hook.Info
	helper RunHookHelper
	name   string

	contextFactory context.Factory
	paths          context.Paths
	context        context.Context
	acquireLock    func(message string) (func(), error)
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
	return fmt.Sprintf("%s%s", rh.info.Kind, suffix)
}

func (rh *runHook) Prepare(state State) (*StateChange, error) {
	if err := rh.checkAlreadyStarted(state); err != nil {
		return nil, err
	}
	name, err := rh.helper.PrepareHook(rh.info)
	if err != nil {
		return nil, err
	}
	ctx, err := rh.contextFactory.NewHookContext(rh.info)
	if err != nil {
		return nil, err
	}
	rh.name = name
	rh.context = ctx
	return &StateChange{
		Kind: RunHook,
		Step: Pending,
		Hook: &rh.info,
	}, nil
}

func (rh *runHook) Execute(state State) (*StateChange, error) {
	message := fmt.Sprintf("running hook %s", rh.name)
	unlock, err := rh.acquireLock(message)
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
		rh.helper.NotifyHookFailed(rh.name, rh.context)
		return nil, ErrHookFailed
	}

	if ranHook {
		logger.Infof("ran %q hook", rh.name)
		rh.helper.NotifyHookCompleted(rh.name, rh.context)
	} else {
		logger.Infof("skipped %q hook (missing)", rh.name)
	}
	return &StateChange{
		Kind: RunHook,
		Step: step,
		Hook: &rh.info,
	}, err
}

func (rh *runHook) Commit(state State) (*StateChange, error) {
	if err := rh.helper.CommitHook(rh.info); err != nil {
		return nil, err
	}
	return &StateChange{
		Kind: Continue,
		Step: Pending,
		Hook: &rh.info,
	}, nil
}

func (rh *runHook) checkAlreadyStarted(state State) error {
	if state.Kind != RunHook {
		return nil
	}
	if *state.Hook != rh.info {
		return nil
	}
	if state.Step == Done {
		return ErrSkipExecute
	}
	return nil
}
