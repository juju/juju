// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package firewaller

import (
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// FirewallerAPI provides access to the Firewaller API facade.
type FirewallerAPI struct {
	*common.LifeGetter
	*common.EnvironWatcher
	*common.AgentEntityWatcher
	*common.UnitsWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter

	st              *state.State
	resources       *common.Resources
	authorizer      common.Authorizer
	getAuthMachines common.GetAuthFunc
	getAuthUnits    common.GetAuthFunc
	getAuthServices common.GetAuthFunc
}

// NewFirewallerAPI creates a new server-side FirewallerAPI facade.
func NewFirewallerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*FirewallerAPI, error) {
	if !authorizer.AuthEnvironManager() {
		// Firewaller must run as environment manager.
		return nil, common.ErrPerm
	}
	// We can get the life of machines, units or services.
	getAuthLife := getAuthFuncForTagKinds(
		names.UnitTagKind,
		names.ServiceTagKind,
		names.MachineTagKind,
	)
	// We can watch units and services only.
	getAuthWatch := getAuthFuncForTagKinds(
		names.UnitTagKind,
		names.ServiceTagKind,
	)
	// We can watch the environ config and read it.
	getAuthEnviron := getAuthFuncForTagKinds("")
	// We can watch environment machines and their instance ids.
	getAuthMachines := getAuthFuncForTagKinds(names.MachineTagKind)
	// We can get opened ports for units.
	getAuthUnits := getAuthFuncForTagKinds(names.UnitTagKind)
	// We can get exposed flag for services.
	getAuthServices := getAuthFuncForTagKinds(names.ServiceTagKind)
	return &FirewallerAPI{
		LifeGetter:             common.NewLifeGetter(st, getAuthLife),
		EnvironWatcher:         common.NewEnvironWatcher(st, resources, getAuthEnviron, getAuthEnviron),
		AgentEntityWatcher:     common.NewAgentEntityWatcher(st, resources, getAuthWatch),
		UnitsWatcher:           common.NewUnitsWatcher(st, resources, getAuthMachines),
		EnvironMachinesWatcher: common.NewEnvironMachinesWatcher(st, resources, getAuthEnviron),
		InstanceIdGetter:       common.NewInstanceIdGetter(st, getAuthMachines),
		st:                     st,
		resources:              resources,
		authorizer:             authorizer,
		getAuthMachines:        getAuthMachines,
		getAuthUnits:           getAuthUnits,
		getAuthServices:        getAuthServices,
	}, nil
}

// OpenedPorts returns the list of opened ports for each given unit.
func (f *FirewallerAPI) OpenedPorts(args params.Entities) (params.PortsResults, error) {
	result := params.PortsResults{
		Results: make([]params.PortsResult, len(args.Entities)),
	}
	canAccess, err := f.getAuthUnits()
	if err != nil {
		return params.PortsResults{}, err
	}
	for i, entity := range args.Entities {
		var unit *state.Unit
		unit, err = f.getUnit(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Ports = unit.OpenedPorts()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetExposed returns the exposed flag value for each given service.
func (f *FirewallerAPI) GetExposed(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := f.getAuthServices()
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		var service *state.Service
		service, err = f.getService(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Result = service.IsExposed()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetAssignedMachine returns the assigned machine tag (if any) for
// each given unit.
func (f *FirewallerAPI) GetAssignedMachine(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := f.getAuthUnits()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		var unit *state.Unit
		unit, err = f.getUnit(canAccess, entity.Tag)
		if err == nil {
			var machineId string
			machineId, err = unit.AssignedMachineId()
			if err == nil {
				result.Results[i].Result = names.MachineTag(machineId)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (f *FirewallerAPI) getEntity(canAccess common.AuthFunc, tag string) (state.Entity, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	return f.st.FindEntity(tag)
}

func (f *FirewallerAPI) getMachine(canAccess common.AuthFunc, tag string) (*state.Machine, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}

func (f *FirewallerAPI) getUnit(canAccess common.AuthFunc, tag string) (*state.Unit, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// unit.
	return entity.(*state.Unit), nil
}

func (f *FirewallerAPI) getService(canAccess common.AuthFunc, tag string) (*state.Service, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// service.
	return entity.(*state.Service), nil
}

// getAuthFuncForTagKinds returns a GetAuthFunc which creates an
// AuthFunc allowing only the given tag kinds and denies all
// others. In the special case where a single empty string is given,
// it's assumed only environment tags are allowed, but not specified
// (for now).
func getAuthFuncForTagKinds(kinds ...string) common.GetAuthFunc {
	getAuthFunc := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			if tag == "" {
				// Assume an empty tag means a missing environment tag.
				// In this case kinds must contain a single item.
				if len(kinds) != 1 {
					return false
				}
				return kinds[0] == ""
			}
			// Allow only the given tag kinds.
			parsedKind, _, err := names.ParseTag(tag, "")
			if err != nil {
				return false
			}
			found := false
			for _, kind := range kinds {
				if kind == parsedKind {
					found = true
				}
			}
			return found
		}, nil
	}
	return getAuthFunc
}
