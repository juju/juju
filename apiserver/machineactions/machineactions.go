// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

// machineactions implements the the apiserver side of
// running actions on machines
package machineactions

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MachineActions", 1, NewMachineActionsAPI)
}

// MachineActionsAPI implements the Logger interface and is the concrete
// implementation of the api end point.
type MachineActionsAPI struct {
	state         *state.State
	resources     *common.Resources
	accessMachine common.AuthFunc
}

// NewMachineActionsAPI creates a new server-side logger API end point.
func NewMachineActionsAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MachineActionsAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &MachineActionsAPI{
		state:         st,
		resources:     resources,
		accessMachine: authorizer.AuthOwner,
	}, nil
}

// Actions returns the Actions by Tags passed and ensures that the machine asking
// for them is the machine that has the actions
func (m *MachineActionsAPI) Actions(args params.Entities) (results params.ActionResults, err error) {
	actionFn := common.AuthAndActionFromTagFn(m.accessMachine, m.state.ActionByTag)
	return common.Actions(args, actionFn)
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (m *MachineActionsAPI) BeginActions(args params.Entities) (results params.ErrorResults, err error) {
	actionFn := common.AuthAndActionFromTagFn(m.accessMachine, m.state.ActionByTag)
	return common.BeginActions(args, actionFn)
}

// FinishActions saves the result of a completed Action
func (m *MachineActionsAPI) FinishActions(args params.ActionExecutionResults) (results params.ErrorResults, err error) {
	actionFn := common.AuthAndActionFromTagFn(m.accessMachine, m.state.ActionByTag)
	return common.FinishActions(args, actionFn)
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a machine.
func (m *MachineActionsAPI) WatchActionNotifications(args params.Entities) (results params.StringsWatchResults, err error) {
	tagToActionReceiver := common.TagToActionReceiverFn(m.state.FindEntity)
	watchOne := common.WatchOneActionReceiverNotifications(tagToActionReceiver, m.resources.Register)
	return common.WatchActionNotifications(args, m.accessMachine, watchOne)
}

// RunningActions lists the actions running for the entities passed in.
// If we end up needing more than ListRunning at some point we could follow/abstract
// what's done in the client actions package.
func (m *MachineActionsAPI) RunningActions(args params.Entities) (params.ActionsByReceivers, error) {
	canAccess := m.accessMachine
	tagToActionReceiver := common.TagToActionReceiverFn(m.state.FindEntity)

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

		results, err := common.ConvertActions(receiver, receiver.RunningActions)
		if err != nil {
			currentResult.Error = common.ServerError(err)
			continue
		}
		currentResult.Actions = results
	}
	return response, nil

}
