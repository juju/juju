// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/rpc/params"
)

type runAction struct {
	action  *uniter.Action
	change  int
	changed chan struct{}
	cancel  chan struct{}

	callbacks     Callbacks
	runnerFactory runner.Factory

	name   string
	runner runner.Runner
	logger Logger
}

// String is part of the Operation interface.
func (ra *runAction) String() string {
	return fmt.Sprintf("run action %s", ra.action.ID())
}

// NeedsGlobalMachineLock is part of the Operation interface.
func (ra *runAction) NeedsGlobalMachineLock() bool {
	return !ra.action.Parallel() || ra.action.ExecutionGroup() != ""
}

// ExecutionGroup is part of the Operation interface.
func (ra *runAction) ExecutionGroup() string {
	return ra.action.ExecutionGroup()
}

// Prepare ensures that the action is valid and can be executed. If not, it
// will return ErrSkipExecute. It preserves any hook recorded in the supplied
// state.
// Prepare is part of the Operation interface.
func (ra *runAction) Prepare(ctx context.Context, state State) (*State, error) {
	ra.changed = make(chan struct{}, 1)
	ra.cancel = make(chan struct{})
	actionID := ra.action.ID()
	rnr, err := ra.runnerFactory.NewActionRunner(ctx, ra.action, ra.cancel)
	if cause := errors.Cause(err); charmrunner.IsBadActionError(cause) {
		if err := ra.callbacks.FailAction(ctx, actionID, err.Error()); err != nil {
			return nil, err
		}
		return nil, ErrSkipExecute
	} else if cause == charmrunner.ErrActionNotAvailable {
		return nil, ErrSkipExecute
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot create runner for action %q", actionID)
	}
	actionData, err := rnr.Context().ActionData()
	if err != nil {
		// this should *really* never happen, but let's not panic
		return nil, errors.Trace(err)
	}
	err = rnr.Context().Prepare(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ra.name = actionData.Name
	ra.runner = rnr
	return stateChange{
		Kind:     RunAction,
		Step:     Pending,
		ActionId: &actionID,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Execute runs the action, and preserves any hook recorded in the supplied state.
// Execute is part of the Operation interface.
func (ra *runAction) Execute(ctx context.Context, state State) (*State, error) {
	message := fmt.Sprintf("running action %s", ra.name)
	if err := ra.callbacks.SetExecutingStatus(message); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	wait := make(chan struct{})
	actionID := ra.action.ID()
	go func() {
		defer close(wait)
		for {
			select {
			case <-done:
				return
			case <-ra.changed:
			}
			status, err := ra.callbacks.ActionStatus(ctx, actionID)
			if err != nil {
				ra.logger.Warningf("unable to get action status for %q: %v", actionID, err)
				continue
			}
			if status == params.ActionAborting {
				ra.logger.Infof("action %s aborting", actionID)
				close(ra.cancel)
				return
			}
		}
	}()

	handlerType, err := ra.runner.RunAction(ctx, ra.name)
	close(done)
	<-wait

	if err != nil {
		// This indicates an actual error -- an action merely failing should
		// be handled inside the Runner, and returned as nil.
		return nil, errors.Annotatef(err, "action %q (via %s) failed", ra.name, handlerType)
	}
	return stateChange{
		Kind:     RunAction,
		Step:     Done,
		ActionId: &actionID,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (ra *runAction) Commit(ctx context.Context, state State) (*State, error) {
	return stateChange{
		Kind: continuationKind(state),
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (ra *runAction) RemoteStateChanged(snapshot remotestate.Snapshot) {
	actionID := ra.action.ID()
	change, ok := snapshot.ActionChanged[actionID]
	if !ok {
		ra.logger.Errorf("action %s missing action changed entry", actionID)
		// Shouldn't happen.
		return
	}
	if ra.change < change {
		ra.change = change
		ra.logger.Errorf("running action %s changed", actionID)
		select {
		case ra.changed <- struct{}{}:
		default:
		}
	}
}

// continuationKind determines what State Kind the operation
// should return after Commit.
func continuationKind(state State) Kind {
	switch {
	case state.Hook != nil:
		return RunHook
	default:
		return Continue
	}
}
