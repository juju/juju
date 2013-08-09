// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the uniter worker.
package uniter

import (
	"fmt"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// UniterAPI implements the API used by the uniter worker.
type UniterAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher

	st         *state.State
	auth       common.Authorizer
	getCanRead common.GetAuthFunc
}

// NewUniterAPI creates a new instance of the Uniter API.
func NewUniterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getCanRead),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanRead),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		st:                 st,
		auth:               authorizer,
		getCanRead:         getCanRead,
	}, nil
}

func (u *UniterAPI) getUnit(tag string) (*state.Unit, error) {
	entity, err := u.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	unit, ok := entity.(*state.Unit)
	if !ok {
		return nil, fmt.Errorf("entity %q is not a unit", tag)
	}
	return unit, nil
}

// PublicAddress returns for each given unit, a pair of the public
// address of the unit and whether it's valid.
func (u *UniterAPI) PublicAddress(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.getCanRead()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	var unit *state.Unit
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canRead(entity.Tag) {
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PublicAddress()
				result.Results[i].Result = address
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetPrivateAddress sets the public address of each of the given units.
func (u *UniterAPI) SetPublicAddress(args params.SetEntityAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := u.getCanRead()
	if err != nil {
		return params.ErrorResults{}, err
	}
	var unit *state.Unit
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canModify(entity.Tag) {
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.SetPublicAddress(entity.Address)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns for each given unit, a pair of the private
// address of the unit and whether it's valid.
func (u *UniterAPI) PrivateAddress(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.getCanRead()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	var unit *state.Unit
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canRead(entity.Tag) {
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PrivateAddress()
				result.Results[i].Result = address
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetPrivateAddress sets the private address of each of the given units.
func (u *UniterAPI) SetPrivateAddress(args params.SetEntityAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := u.getCanRead()
	if err != nil {
		return params.ErrorResults{}, err
	}
	var unit *state.Unit
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canModify(entity.Tag) {
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.SetPrivateAddress(entity.Address)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// TODO(dimitern): Add the other needed API calls.
