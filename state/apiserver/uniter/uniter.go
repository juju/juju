// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface
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

	st           *state.State
	auth         common.Authorizer
	getCanAccess common.GetAuthFunc
}

// NewUniterAPI creates a new instance of the Uniter API.
func NewUniterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanAccess := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanAccess),
		StatusSetter:       common.NewStatusSetter(st, getCanAccess),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanAccess),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanAccess),
		st:                 st,
		auth:               authorizer,
		getCanAccess:       getCanAccess,
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
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
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
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
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
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
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
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.SetPrivateAddress(entity.Address)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPI) ClearResolved(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.ClearResolved()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() on each given unit.
func (u *UniterAPI) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				principal, ok := unit.PrincipalName()
				result.Results[i].Result = principal
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Destroy advances all given Alive units' lifecycles as far as
// possible. See state/Unit.Destroy().
func (u *UniterAPI) Destroy(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.Destroy()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SubordinateNames returns the names of any subordinate units, for each given unit.
func (u *UniterAPI) SubordinateNames(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := u.getCanAccess()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				result.Results[i].Result = unit.SubordinateNames()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmURL returns the charm URL for all given units.
func (u *UniterAPI) CharmURL(args params.Entities) (params.StringBoolResults, error) {
}

// SetCharmURL sets the charm URL for each given unit. An error will
// be returned if a unit is dead, or the charm URL is not know.
func (u *UniterAPI) SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error) {
}

// OpenPort sets the policy of the port with protocol an number to be
// opened, for all given units.
func (u *UniterAPI) OpenPort(args params.EntitiesPorts) (params.ErrorResults, error) {
}

// ClosePort sets the policy of the port with protocol and number to
// be closed, for all given units.
func (u *UniterAPI) ClosePort(args params.EntitiesPorts) (params.ErrorResults, error) {
}

// TODO(dimitern): Add the following needed calls:
// WatchServiceConfig
// WatchService
// WatchServiceRelations
// ServiceLife
// ServiceCharmURL
// GetCharmURL
// GetCharmBundleURL
// GetCharmBundleSha256
// RelationSetDying
// RelationIsImplicit
// GetRelation
// GetRelationSettings
// RelationEnterScope
// RelationLeaveScope
// WatchRelation
// EndpointImplementedBy
// EnvironUUID
