// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Firewaller", 1, NewFirewallerAPIV1)
}

// FirewallerAPIV1 provides access to the Firewaller API facade,
// version 1. In this version the firewaller watches new-style port
// ranges opened on machines, rather than units. Other changes:
// - OpenedPorts(taking unit tags) is removed.
// - Watch() is no longer allowed for unit tags.
// + GetMachineActiveNetworks() is added
// + GetMachinePorts() is added.
// + WatchOpenedPorts() is added (taking environ tags).
type FirewallerAPIV1 struct {
	FirewallerAPIBase

	accessMachine common.GetAuthFunc
	accessEnviron common.GetAuthFunc
}

// NewFirewallerAPIV1 creates a new server-side FirewallerAPI facade,
// version 1.
func NewFirewallerAPIV1(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*FirewallerAPIV1, error) {
	if !authorizer.AuthEnvironManager() {
		// Firewaller must run as environment manager.
		return nil, common.ErrPerm
	}
	// Set up the various authorization checkers.
	accessEnviron := getAuthFuncForTagKind(names.EnvironTagKind)
	accessUnit := getAuthFuncForTagKind(names.UnitTagKind)
	accessService := getAuthFuncForTagKind(names.ServiceTagKind)
	accessMachine := getAuthFuncForTagKind(names.MachineTagKind)
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

	return &FirewallerAPIV1{
		FirewallerAPIBase: FirewallerAPIBase{
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
		},
		accessMachine: accessMachine,
		accessEnviron: accessEnviron,
	}, nil
}

// WatchOpenedPorts returns a new StringsWatcher for each given
// environment tag.
func (f *FirewallerAPIV1) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
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

func (f *FirewallerAPIV1) watchOneEnvironOpenedPorts(tag names.Tag) (string, []string, error) {
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
func (f *FirewallerAPIV1) GetMachinePorts(args params.MachinePortsParams) (params.MachinePortsResults, error) {
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
			portRanges := ports.AllPortRanges()
			for portRange, unitName := range portRanges {
				unitTag := names.NewUnitTag(unitName).String()
				result.Results[i].Ports = append(result.Results[i].Ports,
					params.MachinePortRange{
						UnitTag:   unitTag,
						PortRange: portRange,
					})
			}
		}
	}
	return result, nil
}

// GetMachineActiveNetworks returns the tags of the all networks the
// each given machine has open ports on.
func (f *FirewallerAPIV1) GetMachineActiveNetworks(args params.Entities) (params.StringsResults, error) {
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
			networkName, err := port.NetworkName()
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				break
			}
			networkTag := names.NewNetworkTag(networkName).String()
			result.Results[i].Result = append(result.Results[i].Result, networkTag)
		}
	}
	return result, nil
}

func (f *FirewallerAPIV1) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}
