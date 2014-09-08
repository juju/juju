// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Machiner", 0, NewMachinerAPIV0)
	common.RegisterStandardFacade("Machiner", 1, NewMachinerAPIV1)
}

// MachinerAPIV0 implements version 0 of the Machiner API.
type MachinerAPIV0 struct {
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

// NewMachinerAPIV0 creates a new instance of the Machiner API V0.
func NewMachinerAPIV0(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MachinerAPIV0, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanModify := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &MachinerAPIV0{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getCanModify),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanModify),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		st:                 st,
		auth:               authorizer,
		getCanModify:       getCanModify,
		getCanRead:         getCanRead,
	}, nil
}

func (api *MachinerAPIV0) getMachine(tag names.Tag) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

// SetMachineAddresses sets the given list of addresses for each given machine tag.
func (api *MachinerAPIV0) SetMachineAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}
	for i, arg := range args.MachineAddresses {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canModify(tag) {
			var m *state.Machine
			m, err = api.getMachine(tag)
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

// MachinerAPIV1 implements version 1 of the Machiner API.
type MachinerAPIV1 struct {
	*MachinerAPIV0
}

// NewMachinerAPIV1 creates a new instance of the Machiner API V1.
func NewMachinerAPIV1(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MachinerAPIV1, error) {
	m0, err := NewMachinerAPIV0(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &MachinerAPIV1{m0}, nil
}

// GetMachines returns information about all machined identified
// by the passed tags.
func (api *MachinerAPIV1) GetMachines(args params.GetMachinesV1) (params.GetMachinesResultsV1, error) {
	results := params.GetMachinesResultsV1{
		Machines: make([]params.GetMachinesResultV1, len(args.Tags)),
	}
	canRead, err := api.getCanRead()
	if err != nil {
		return results, err
	}
	for i, atag := range args.Tags {
		results.Machines[i].Tag = atag
		tag, err := names.ParseMachineTag(atag)
		if err != nil {
			results.Machines[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canRead(tag) {
			results.Machines[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		m, err := api.getMachine(tag)
		if err != nil {
			results.Machines[i].Error = common.ServerError(err)
			continue
		}
		isManual, err := m.IsManual()
		if err != nil {
			results.Machines[i].Error = common.ServerError(err)
			continue
		}
		results.Machines[i].Life = params.Life(m.Life().String())
		results.Machines[i].IsManual = isManual
	}
	return results, nil
}
