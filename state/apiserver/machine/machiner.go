// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

func init() {
	common.RegisterStandardFacade("Machiner", 0, NewMachinerAPI)
}

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser

	st           *state.State
	auth         common.Authorizer
	getCanModify common.GetAuthFunc
	getCanRead   common.GetAuthFunc
}

// NewMachinerAPI creates a new instance of the Machiner API.
func NewMachinerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanModify := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &MachinerAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getCanModify),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanModify),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		st:                 st,
		auth:               authorizer,
		getCanModify:       getCanModify,
	}, nil
}

func (api *MachinerAPI) getMachine(tag string) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

// GetMachines implements the API call GetMachines.
func (api *MachinerAPI) GetMachines(args params.GetMachinesV0) (params.GetMachinesResultsV0, error) {
	results := params.GetMachinesResultsV0{
		Machines: make([]params.GetMachinesResultV0, len(args.Tags)),
	}
	for i, tag := range args.Tags {
		err := common.ErrPerm
		var m *state.Machine
		m, err = api.getMachine(tag)
		if err == nil {
			isManual, err := m.IsManual()
			if err == nil {
				life := m.Life()
				results.Machines[i].Tag = tag
				results.Machines[i].Life = params.Life(life.String())
				results.Machines[i].IsManual = isManual
			}
		} else if errors.IsNotFound(err) {
			err = common.ErrPerm
		}
		results.Machines[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *MachinerAPI) SetMachineAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.MachineAddresses {
		err := common.ErrPerm
		if canModify(arg.Tag) {
			var m *state.Machine
			m, err = api.getMachine(arg.Tag)
			if err == nil {
				err = m.SetMachineAddresses(arg.Addresses...)
			} else if errors.IsNotFound(err) {
				err = common.ErrPerm
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}
