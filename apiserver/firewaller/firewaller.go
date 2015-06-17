// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	// Version 0 is no longer supported.
	common.RegisterStandardFacade("Firewaller", 1, NewFirewallerAPI)
}

// FirewallerAPI provides access to the Firewaller API facade.
type FirewallerAPI struct {
	*common.LifeGetter
	*common.EnvironWatcher
	*common.AgentEntityWatcher
	*common.UnitsWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter

	st            *state.State
	resources     *common.Resources
	authorizer    common.Authorizer
	accessUnit    common.GetAuthFunc
	accessService common.GetAuthFunc
	accessMachine common.GetAuthFunc
	accessEnviron common.GetAuthFunc
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
	// Set up the various authorization checkers.
	accessEnviron := common.AuthFuncForTagKind(names.EnvironTagKind)
	accessUnit := common.AuthFuncForTagKind(names.UnitTagKind)
	accessService := common.AuthFuncForTagKind(names.ServiceTagKind)
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	accessUnitServiceOrMachine := common.AuthEither(accessUnitOrService, accessMachine)

	// Life() is supported for units, services or machines.
	lifeGetter := common.NewLifeGetter(
		st,
		accessUnitServiceOrMachine,
	)
	// EnvironConfig() and WatchForEnvironConfigChanges() are allowed
	// with unrestriced access.
	environWatcher := common.NewEnvironWatcher(
		st,
		resources,
		authorizer,
	)
	// Watch() is supported for services only.
	entityWatcher := common.NewAgentEntityWatcher(
		st,
		resources,
		accessService,
	)
	// WatchUnits() is supported for machines.
	unitsWatcher := common.NewUnitsWatcher(st,
		resources,
		accessMachine,
	)
	// WatchEnvironMachines() is allowed with unrestricted access.
	machinesWatcher := common.NewEnvironMachinesWatcher(
		st,
		resources,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		st,
		accessMachine,
	)

	return &FirewallerAPI{
		LifeGetter:             lifeGetter,
		EnvironWatcher:         environWatcher,
		AgentEntityWatcher:     entityWatcher,
		UnitsWatcher:           unitsWatcher,
		EnvironMachinesWatcher: machinesWatcher,
		InstanceIdGetter:       instanceIdGetter,
		st:                     st,
		resources:              resources,
		authorizer:             authorizer,
		accessUnit:             accessUnit,
		accessService:          accessService,
		accessMachine:          accessMachine,
		accessEnviron:          accessEnviron,
	}, nil
}

// WatchOpenedPorts returns a new StringsWatcher for each given
// environment tag.
func (f *FirewallerAPI) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := f.accessEnviron()
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canWatch(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		watcherId, initial, err := f.watchOneEnvironOpenedPorts(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (f *FirewallerAPI) watchOneEnvironOpenedPorts(tag names.Tag) (string, []string, error) {
	// NOTE: tag is ignored, as there is only one environment in the
	// state DB. Once this changes, change the code below accordingly.
	watch := f.st.WatchOpenedPorts()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// GetMachinePorts returns the port ranges opened on a machine for the
// specified network as a map mapping port ranges to the tags of the
// units that opened them.
func (f *FirewallerAPI) GetMachinePorts(args params.MachinePortsParams) (params.MachinePortsResults, error) {
	result := params.MachinePortsResults{
		Results: make([]params.MachinePortsResult, len(args.Params)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.MachinePortsResults{}, err
	}
	for i, param := range args.Params {
		machineTag, err := names.ParseMachineTag(param.MachineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		networkTag, err := names.ParseNetworkTag(param.NetworkTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ports, err := machine.OpenedPorts(networkTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if ports != nil {
			portRangeMap := ports.AllPortRanges()
			var portRanges []network.PortRange
			for portRange := range portRangeMap {
				portRanges = append(portRanges, portRange)
			}
			network.SortPortRanges(portRanges)

			for _, portRange := range portRanges {
				unitTag := names.NewUnitTag(portRangeMap[portRange]).String()
				result.Results[i].Ports = append(result.Results[i].Ports,
					params.MachinePortRange{
						UnitTag:   unitTag,
						PortRange: params.FromNetworkPortRange(portRange),
					})
			}
		}
	}
	return result, nil
}

// GetMachineActiveNetworks returns the tags of the all networks the
// each given machine has open ports on.
func (f *FirewallerAPI) GetMachineActiveNetworks(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ports, err := machine.AllPorts()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		for _, port := range ports {
			networkTag := names.NewNetworkTag(port.NetworkName()).String()
			result.Results[i].Result = append(result.Results[i].Result, networkTag)
		}
	}
	return result, nil
}

// GetExposed returns the exposed flag value for each given service.
func (f *FirewallerAPI) GetExposed(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := f.accessService()
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		service, err := f.getService(canAccess, tag)
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
	canAccess, err := f.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := f.getUnit(canAccess, tag)
		if err == nil {
			var machineId string
			machineId, err = unit.AssignedMachineId()
			if err == nil {
				result.Results[i].Result = names.NewMachineTag(machineId).String()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (f *FirewallerAPI) getEntity(canAccess common.AuthFunc, tag names.Tag) (state.Entity, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	return f.st.FindEntity(tag)
}

func (f *FirewallerAPI) getUnit(canAccess common.AuthFunc, tag names.UnitTag) (*state.Unit, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// unit.
	return entity.(*state.Unit), nil
}

func (f *FirewallerAPI) getService(canAccess common.AuthFunc, tag names.ServiceTag) (*state.Service, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// service.
	return entity.(*state.Service), nil
}

func (f *FirewallerAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}
