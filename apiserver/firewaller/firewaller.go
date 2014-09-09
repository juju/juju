// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package firewaller

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Firewaller", 0, NewFirewallerAPI)
}

// FirewallerAPI provides access to the Firewaller API facade.
type FirewallerAPI struct {
	*common.LifeGetter
	*common.EnvironWatcher
	*common.AgentEntityWatcher
	*common.UnitsWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter
	*OpenedPortsWatcherFactory
	st            *state.State
	resources     *common.Resources
	authorizer    common.Authorizer
	accessUnit    common.GetAuthFunc
	accessMachine common.GetAuthFunc
	accessService common.GetAuthFunc
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
	accessUnit := getAuthFuncForTagKind(names.UnitTagKind)
	accessService := getAuthFuncForTagKind(names.ServiceTagKind)
	accessMachine := getAuthFuncForTagKind(names.MachineTagKind)
	accessEnviron := getAuthFuncForTagKind(names.EnvironTagKind)
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
	// Watch() is supported for units or services.
	entityWatcher := common.NewAgentEntityWatcher(
		st,
		resources,
		accessUnitOrService,
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
	openedPortsWatcherFactory := NewOpenedPortsWatcherFactory(
		st,
		resources,
		accessEnviron,
	)
	return &FirewallerAPI{
		LifeGetter:                lifeGetter,
		EnvironWatcher:            environWatcher,
		AgentEntityWatcher:        entityWatcher,
		UnitsWatcher:              unitsWatcher,
		EnvironMachinesWatcher:    machinesWatcher,
		InstanceIdGetter:          instanceIdGetter,
		OpenedPortsWatcherFactory: openedPortsWatcherFactory,
		st:            st,
		resources:     resources,
		authorizer:    authorizer,
		accessUnit:    accessUnit,
		accessMachine: accessMachine,
		accessService: accessService,
	}, nil
}

// OpenedPorts returns the list of opened ports for each given unit.
func (f *FirewallerAPI) OpenedPorts(args params.Entities) (params.PortsResults, error) {
	result := params.PortsResults{
		Results: make([]params.PortsResult, len(args.Entities)),
	}
	canAccess, err := f.accessUnit()
	if err != nil {
		return params.PortsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := f.getUnit(canAccess, tag)
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

// GetMachinePorts returns the port ranges opened on a machine for the specified network as a map mapping
// port ranges to the tags of the units that opened them.
func (f *FirewallerAPI) GetMachinePorts(args params.MachinePortsParams) (params.MachinePortsResults, error) {
	result := params.MachinePortsResults{
		Results: make([]params.MachinePortsResult, len(args.Params)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.MachinePortsResults{}, err
	}
	for i, param := range args.Params {
		machineTag, err := names.ParseMachineTag(param.Machine)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		net, err := names.ParseNetworkTag(param.Network)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ports, err := machine.OpenedPorts(net.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if ports != nil {
			portRanges := ports.AllPortRanges()
			for portRange, unitTag := range portRanges {
				result.Results[i].Ports = append(result.Results[i].Ports,
					params.MachinePortDef{
						Range: portRange,
						Unit:  params.Entity{Tag: unitTag.String()},
					})
			}
		}
	}
	return result, nil
}

// GetMachineActiveNetworks returns the names of the networks the machine has open ports on.
func (f *FirewallerAPI) GetMachineActiveNetworks(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		var machine *state.Machine
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machine, err = f.getMachine(canAccess, machineTag)
		if err == nil {
			ports, err := machine.AllPorts()
			if err == nil {
				for _, port := range ports {
					networkName, err := port.NetworkName()
					if err == nil {
						result.Results[i].Result = append(result.Results[i].Result, networkName)
					}
				}
			}
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

func (f *FirewallerAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
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

// getAuthFuncForTagKind returns a GetAuthFunc which creates an
// AuthFunc allowing only the given tag kind and denies all
// others. In the special case where a single empty string is given,
// it's assumed only environment tags are allowed, but not specified
// (for now).
func getAuthFuncForTagKind(kind string) common.GetAuthFunc {
	return func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// Allow only the given tag kind.
			return tag.Kind() == kind
		}, nil
	}
}
