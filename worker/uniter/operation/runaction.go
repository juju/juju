// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
)

type runAction struct {
	actionId string

	change  int
	changed chan struct{}
	cancel  chan struct{}

	callbacks     Callbacks
	runnerFactory runner.Factory

	name   string
	runner runner.Runner

	RequiresMachineLock
}

// String is part of the Operation interface.
func (ra *runAction) String() string {
	return fmt.Sprintf("run action %s", ra.actionId)
}

// Prepare ensures that the action is valid and can be executed. If not, it
// will return ErrSkipExecute. It preserves any hook recorded in the supplied
// state.
// Prepare is part of the Operation interface.
func (ra *runAction) Prepare(state State) (*State, error) {
	ra.changed = make(chan struct{}, 1)
	ra.cancel = make(chan struct{})
	rnr, err := ra.runnerFactory.NewActionRunner(ra.actionId, ra.cancel)
	if cause := errors.Cause(err); charmrunner.IsBadActionError(cause) {
		if err := ra.callbacks.FailAction(ra.actionId, err.Error()); err != nil {
			return nil, err
		}
		return nil, ErrSkipExecute
	} else if cause == charmrunner.ErrActionNotAvailable {
		return nil, ErrSkipExecute
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot create runner for action %q", ra.actionId)
	}
	actionData, err := rnr.Context().ActionData()
	if err != nil {
		// this should *really* never happen, but let's not panic
		return nil, errors.Trace(err)
	}
	err = rnr.Context().Prepare()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ra.name = actionData.Name
	ra.runner = rnr
	return stateChange{
		Kind:     RunAction,
		Step:     Pending,
		ActionId: &ra.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Execute runs the action, and preserves any hook recorded in the supplied state.
// Execute is part of the Operation interface.
func (ra *runAction) Execute(state State) (*State, error) {
	message := fmt.Sprintf("running action %s", ra.name)
	if err := ra.callbacks.SetExecutingStatus(message); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	wait := make(chan struct{})
	go func() {
		defer close(wait)
		for {
			select {
			case <-done:
				return
			case <-ra.changed:
			}
			status, err := ra.callbacks.ActionStatus(ra.actionId)
			if err != nil {
				logger.Warningf("unable to get action status for %q: %v", ra.actionId, err)
				continue
			}
			if status == params.ActionAborting {
				logger.Infof("action %s aborting", ra.actionId)
				close(ra.cancel)
				return
			}
		}
	}()

	handlerType, err := ra.runner.RunAction(ra.name)
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
		ActionId: &ra.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (ra *runAction) Commit(state State) (*State, error) {
	return stateChange{
		Kind: continuationKind(state),
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (ra *runAction) RemoteStateChanged(snapshot remotestate.Snapshot) {
	change, ok := snapshot.ActionChanged[ra.actionId]
	if !ok {
		logger.Errorf("action %s missing action changed entry", ra.actionId)
		// Shouldn't happen.
		return
	}
	if ra.change < change {
		ra.change = change
		logger.Errorf("running action %s changed", ra.actionId)
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
