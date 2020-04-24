// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

// machineactions implements the the apiserver side of
// running actions on machines
package machineactions

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
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
	backend       Backend
	resources     facade.Resources
	accessMachine common.AuthFunc
}

// NewFacade creates a new server-side machineactions API end point.
func NewFacade(
	backend Backend,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*Facade, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &Facade{
		backend:       backend,
		resources:     resources,
		accessMachine: authorizer.AuthOwner,
	}, nil
}

// Actions returns the Actions by Tags passed and ensures that the machine asking
// for them is the machine that has the actions
func (f *Facade) Actions(args params.Entities) params.ActionResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.Actions(args, actionFn)
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (f *Facade) BeginActions(args params.Entities) params.ErrorResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.BeginActions(args, actionFn)
}

// FinishActions saves the result of a completed Action
func (f *Facade) FinishActions(args params.ActionExecutionResults) params.ErrorResults {
	actionFn := common.AuthAndActionFromTagFn(f.accessMachine, f.backend.ActionByTag)
	return common.FinishActions(args, actionFn)
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a machine.
func (f *Facade) WatchActionNotifications(args params.Entities) params.StringsWatchResults {
	tagToActionReceiver := f.backend.TagToActionReceiverFn(f.backend.FindEntity)
	watchOne := common.WatchPendingActionsForReceiver(tagToActionReceiver, f.resources.Register)
	return common.WatchActionNotifications(args, f.accessMachine, watchOne)
}

// RunningActions lists the actions running for the entities passed in.
// If we end up needing more than ListRunning at some point we could follow/abstract
// what's done in the client actions package.
func (f *Facade) RunningActions(args params.Entities) params.ActionsByReceivers {
	canAccess := f.accessMachine
	tagToActionReceiver := f.backend.TagToActionReceiverFn(f.backend.FindEntity)

	response := params.ActionsByReceivers{
		Actions: make([]params.ActionsByReceiver, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		currentResult := &response.Actions[i]
		receiver, err := tagToActionReceiver(entity.Tag)
		if err != nil {
			currentResult.Error = common.ServerError(common.ErrBadId)
			continue
		}
		currentResult.Receiver = receiver.Tag().String()

		if !canAccess(receiver.Tag()) {
			currentResult.Error = common.ServerError(common.ErrPerm)
			continue
		}

		results, err := f.backend.ConvertActions(receiver, receiver.RunningActions)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		currentResult.Actions = results
	}

	return response
}
