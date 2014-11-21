// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/context"
)

type runAction struct {
	state     *apiuniter.State
	actionId  string
	actionTag names.ActionTag
	name      string

	contextFactory context.Factory
	paths          context.Paths
	context        context.Context
	acquireLock    func(message string) (func(), error)
}

func (ra *runAction) String() string {
	return fmt.Sprintf("run action %s", ra.actionId)
}

func (ra *runAction) Prepare(state State) (*State, error) {
	ctx, err := ra.contextFactory.NewActionContext(ra.actionId)
	if cause := errors.Cause(err); context.IsBadActionError(cause) {
		print(fmt.Sprintf("%v\n%v\n", err, cause))
		finishErr := ra.state.ActionFinish(ra.actionTag, params.ActionFailed, nil, err.Error())
		if params.IsCodeNotFoundOrCodeUnauthorized(finishErr) {
			return nil, ErrSkipExecute
		} else if finishErr != nil {
			return nil, finishErr
		}
		return nil, ErrSkipExecute
	} else if cause == context.ErrActionNotAvailable {
		return nil, ErrSkipExecute
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot create context for action %q", ra.actionId)
	}
	ra.name, err = ctx.ActionName()
	if err != nil {
		// this should *really* never happen, but let's not panic
		return nil, errors.Trace(err)
	}
	ra.context = ctx
	return stateChange{
		Kind:     RunAction,
		Step:     Pending,
		ActionId: &ra.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

func (ra *runAction) Execute(state State) (*State, error) {
	message := fmt.Sprintf("running action %s", ra.name)
	unlock, err := ra.acquireLock(message)
	if err != nil {
		return nil, err
	}
	defer unlock()

	runner := context.NewRunner(ra.context, ra.paths)
	err = runner.RunAction(ra.name)
	if err != nil {
		// This indicates an actual error -- an action merely failing should
		// be handled inside the Runner, and returned as nil.
		return nil, errors.Annotatef(err, "running action %q", ra.name)
	}
	return stateChange{
		Kind:     RunAction,
		Step:     Done,
		ActionId: &ra.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

func (ra *runAction) Commit(state State) (*State, error) {
	return stateChange{
		Kind: Continue,
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}
