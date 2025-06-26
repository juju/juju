// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type Backend interface {
	ActionByTag(tag names.ActionTag) (state.Action, error)
	FindEntity(tag names.Tag) (state.Entity, error)
	TagToActionReceiverFn(findEntity func(names.Tag) (state.Entity, error)) func(string) (state.ActionReceiver, error)
	ConvertActions(ar state.ActionReceiver, fn common.GetActionsFn) ([]params.ActionResult, error)
}

// Facade implements the machineactions interface and is the concrete
// implementation of the api end point.
type Facade struct {
	backend         Backend
	watcherRegistry facade.WatcherRegistry
	accessMachine   common.AuthFunc
}

// NewFacade creates a new server-side machineactions API end point.
func NewFacade(
	backend Backend,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
) (*Facade, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		backend:         backend,
		watcherRegistry: watcherRegistry,
		accessMachine:   authorizer.AuthOwner,
	}, nil
}

// Actions returns the Actions by Tags passed and ensures that the machine asking
// for them is the machine that has the actions
func (f *Facade) Actions(ctx context.Context, args params.Entities) params.ActionResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.Actions(args, actionFn)
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (f *Facade) BeginActions(ctx context.Context, args params.Entities) params.ErrorResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.BeginActions(args, actionFn)
}

// FinishActions saves the result of a completed Action
func (f *Facade) FinishActions(ctx context.Context, args params.ActionExecutionResults) params.ErrorResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.FinishActions(args, actionFn)
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a machine.
func (f *Facade) WatchActionNotifications(ctx context.Context, args params.Entities) params.StringsWatchResults {
	results := make([]params.StringsWatchResult, len(args.Entities))

	for i := range args.Entities {
		result := &results[i]

		// We need a notify watcher for each item, otherwise during a migration
		// a 3.x agent will bounce and will not be able to continue. By
		// providing a watcher which does nothing, we can ensure that the 3.x
		// agent will continue to work.
		watcher := watcher.TODO[[]string]()
		id, _, err := internal.EnsureRegisterWatcher(ctx, f.watcherRegistry, watcher)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			continue
		}
		result.StringsWatcherId = id
	}
	return params.StringsWatchResults{Results: results}
}

// RunningActions lists the actions running for the entities passed in.
// If we end up needing more than ListRunning at some point we could follow/abstract
// what's done in the client actions package.
func (f *Facade) RunningActions(ctx context.Context, args params.Entities) params.ActionsByReceivers {
	canAccess := f.accessMachine
	tagToActionReceiver := f.backend.TagToActionReceiverFn(f.backend.FindEntity)

	response := params.ActionsByReceivers{
		Actions: make([]params.ActionsByReceiver, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		currentResult := &response.Actions[i]
		receiver, err := tagToActionReceiver(entity.Tag)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrBadId)
			continue
		}
		currentResult.Receiver = receiver.Tag().String()

		if !canAccess(receiver.Tag()) {
			currentResult.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		results, err := f.backend.ConvertActions(receiver, receiver.RunningActions)
		if err != nil {
			currentResult.Error = apiservererrors.ServerError(err)
			continue
		}
		currentResult.Actions = results
	}

	return response
}
