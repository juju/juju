// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

var logger = loggo.GetLogger("juju.worker.uniter.actions")

type actionsResolver struct {
	opFactory       operation.Factory
	settingsVersion int
}

func NewResolver(opFactory operation.Factory) resolver.Resolver {
	return &actionsResolver{opFactory: opFactory}
}

func (r *actionsResolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	// TODO Probably need this????
	if !opState.Installed {
		return nil, resolver.ErrNoOperation
	}

	// TODO the completedAction and runningAction
	// need to be reported back to the state server
	// somehow.
	if opState.Kind == operation.Continue {
		//completedAction := opState.ActionId
	}
	if opState.Kind == operation.RunHook {
		//runningAction := opState.ActionId
	}
	// TODO How do we pick what to do next.
	// TODO how do we report back up to the state server
	// what is being done
	actionID := remoteState.Actions[0]
	action, err := r.opFactory.NewAction(actionID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return action, nil
}
