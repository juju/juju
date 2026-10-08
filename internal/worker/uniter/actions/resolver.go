// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type actionsResolver struct {
	logger          logger.Logger
	actionCompleted func(id string)
}

// NewResolver returns a new resolver with determines which action related operation
// should be run based on local and remote uniter states. When an action
// operation is committed, the ID of the action is passed to the
// "actionCompleted" callback.
func NewResolver(logger logger.Logger, actionCompleted func(string)) resolver.Resolver {
	return &actionsResolver{logger: logger, actionCompleted: actionCompleted}
}

// NextOp implements the resolver.Resolver interface.
func (r *actionsResolver) NextOp(
	ctx context.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (op operation.Operation, err error) {
	// If there are no operation left to be run, then we cannot return the
	// error signaling such here, we must first check to see if an action is
	// already running (that has been interrupted) before we declare that
	// there is nothing to do.
	var nextActionId string
	if len(remoteState.ActionsPending) > 0 {
		nextActionId = remoteState.ActionsPending[0]
	} else {
		r.logger.Debugf(ctx, "no next action from pending=%v", remoteState.ActionsPending)
	}

	defer func() {
		if errors.Cause(err) == charmrunner.ErrActionNotAvailable {
			if localState.Step == operation.Pending && localState.ActionId != nil {
				r.logger.Infof(ctx, "found missing not yet started action %v; running fail action", *localState.ActionId)
				op, err = r.completeOnCommit(ctx, opFactory.NewFailAction, *localState.ActionId)
			} else if nextActionId != "" {
				r.logger.Infof(ctx, "found missing incomplete action %v; running fail action", nextActionId)
				op, err = r.completeOnCommit(ctx, opFactory.NewFailAction, nextActionId)
			} else {
				err = resolver.ErrNoOperation
			}
		}
	}()

	switch localState.Kind {
	case operation.RunHook:
		// We can still run actions if the unit is in a hook error state.
		if localState.Step == operation.Pending && nextActionId != "" {
			return r.completeOnCommit(ctx, opFactory.NewAction, nextActionId)
		}
	case operation.RunAction:
		if localState.Hook != nil {
			r.logger.Infof(ctx, "found incomplete action %v; ignoring", localState.ActionId)
			r.logger.Infof(ctx, "recommitting prior %q hook", localState.Hook.Kind)
			return opFactory.NewSkipHook(*localState.Hook)
		}

		r.logger.Infof(ctx, "%q hook is nil, so running action %v", operation.RunAction, nextActionId)
		// If the next action is the same as what the uniter is
		// currently running then this means that the uniter was
		// some how interrupted (killed) when running the action
		// and before updating the remote state to indicate that
		// the action was completed. The only safe thing to do
		// is fail the action, since rerunning an arbitrary
		// command can potentially be hazardous.
		if nextActionId == *localState.ActionId {
			r.logger.Debugf(ctx, "unit agent was interrupted while running action %v", *localState.ActionId)
			return r.completeOnCommit(ctx, opFactory.NewFailAction, *localState.ActionId)
		}

		// If the next action is different then what the uniter
		// is currently running, then the uniter may have been
		// interrupted while running the action but the remote
		// state was updated. Thus, the semantics of
		// (re)preparing the running operation should move the
		// uniter's state along safely. Thus, we return the
		// running action.
		return r.completeOnCommit(ctx, opFactory.NewAction, *localState.ActionId)
	case operation.Continue:
		if nextActionId != "" {
			return r.completeOnCommit(ctx, opFactory.NewAction, nextActionId)
		}
	}
	return nil, resolver.ErrNoOperation
}

// completeOnCommit returns the operation that newOp creates for the
// action, which passes the action ID to the "actionCompleted" callback
// when it is committed.
func (r *actionsResolver) completeOnCommit(ctx context.Context, newOp func(context.Context, string) (operation.Operation, error), id string) (operation.Operation, error) {
	op, err := newOp(ctx, id)
	if err != nil {
		return nil, err
	}
	return &actionCompleter{op, func() { r.actionCompleted(id) }}, nil
}

type actionCompleter struct {
	operation.Operation
	actionCompleted func()
}

func (c *actionCompleter) Commit(ctx context.Context, st operation.State) (*operation.State, error) {
	result, err := c.Operation.Commit(ctx, st)
	if err == nil {
		c.actionCompleted()
	}
	return result, err
}

// WrappedOperation is part of the WrappedOperation interface.
func (c *actionCompleter) WrappedOperation() operation.Operation {
	return c.Operation
}
