// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

var logger = loggo.GetLogger("juju.worker.uniter.actions")

type actionsResolver struct{}

// NewResolver returns a new resolver with determines which action related operation
// should be run based on local and remote uniter states.
//
// TODO(axw) 2015-10-27 #1510333
// Use the same method as in the runcommands resolver
// for updating the remote state snapshot when an
// action is completed.
func NewResolver() resolver.Resolver {
	return &actionsResolver{}
}

func nextAction(pendingActions []string, completedActions map[string]struct{}) (string, error) {
	for _, action := range pendingActions {
		if _, ok := completedActions[action]; !ok {
			return action, nil
		}
	}
	return "", resolver.ErrNoOperation
}

// NextOp implements the resolver.Resolver interface.
func (r *actionsResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	nextAction, err := nextAction(remoteState.Actions, localState.CompletedActions)
	if err != nil {
		return nil, err
	}
	switch localState.Kind {
	case operation.RunHook:
		// We can still run actions if the unit is in a hook error state.
		if localState.Step == operation.Pending {
			return opFactory.NewAction(nextAction)
		}
	case operation.RunAction:
		// TODO(fwereade): we *should* handle interrupted actions, and make sure
		// they're marked as failed, but that's not for now.
		if localState.Hook != nil {
			logger.Infof("found incomplete action %q; ignoring", localState.ActionId)
			logger.Infof("recommitting prior %q hook", localState.Hook.Kind)
			return opFactory.NewSkipHook(*localState.Hook)
		} else {
			logger.Infof("%q hook is nil", operation.RunAction)
		}
	case operation.Continue:
		return opFactory.NewAction(nextAction)
	}
	return nil, resolver.ErrNoOperation
}
