// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 1.
package uniter

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
	common.RegisterStandardFacade("Uniter", 1, NewUniterAPIV1)
}

// UniterAPI implements the API version 1, used by the uniter worker.
type UniterAPIV1 struct {
	uniterBaseAPI

	accessMachine common.GetAuthFunc
}

// NewUniterAPIV1 creates a new instance of the Uniter API, version 1.
func NewUniterAPIV1(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV1, error) {
	baseAPI, err := newUniterBaseAPI(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	accessMachine := func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineId, err := entity.AssignedMachineId()
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineTag := names.NewMachineTag(machineId)
			return func(tag names.Tag) bool {
				return tag == machineTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
	}
	return &UniterAPIV1{
		uniterBaseAPI: *baseAPI,

		accessMachine: accessMachine,
	}, nil
}

// AllMachinePorts returns all opened port ranges for each given
// machine (on all networks).
func (u *UniterAPIV1) AllMachinePorts(args params.Entities) (params.MachinePortsResults, error) {
	result := params.MachinePortsResults{
		Results: make([]params.MachinePortsResult, len(args.Entities)),
	}
	canAccess, err := u.accessMachine()
	if err != nil {
		return params.MachinePortsResults{}, err
	}
	for i, entity := range args.Entities {
		result.Results[i] = u.getOneMachinePorts(canAccess, entity.Tag)
	}
	return result, nil
}

// ServiceOwner returns the owner user for each given service tag.
func (u *UniterAPIV1) ServiceOwner(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		service, err := u.getService(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].Result = service.GetOwnerTag()
	}
	return result, nil
}

// AssignedMachine returns the machine tag for each given unit tag, or
// an error satisfying params.IsCodeNotAssigned when a unit has no
// assigned machine.
func (u *UniterAPIV1) AssignedMachine(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		} else {
			result.Results[i].Result = names.NewMachineTag(machineId).String()
		}
	}
	return result, nil
}

// WatchStorageInstances returns a StringsWatcher for observing changes to a
// unit's storage instances.
func (u *UniterAPIV1) WatchStorageInstances(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	one := func(entity params.Entity) (string, []string, error) {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, common.ErrPerm
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			return "", nil, common.ErrPerm
		}
		watch := unit.WatchStorageInstances()
		if changes, ok := <-watch.Changes(); ok {
			return u.resources.Register(watch), changes, nil
		}
		return "", nil, watcher.EnsureErr(watch)
	}
	for i, entity := range args.Entities {
		id, changes, err := one(entity)
		if err == nil {
			results.Results[i].StringsWatcherId = id
			results.Results[i].Changes = changes
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (u *UniterAPIV1) getMachine(tag names.MachineTag) (*state.Machine, error) {
	return u.st.Machine(tag.Id())
}

func (u *UniterAPIV1) getOneMachinePorts(canAccess common.AuthFunc, machineTag string) params.MachinePortsResult {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(common.ErrPerm)}
	}
	if !canAccess(tag) {
		return params.MachinePortsResult{Error: common.ServerError(common.ErrPerm)}
	}
	machine, err := u.getMachine(tag)
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(err)}
	}
	allPorts, err := machine.AllPorts()
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(err)}
	}
	var resultPorts []params.MachinePortRange
	for _, ports := range allPorts {
		// AllPortRanges gives a map, but apis require a stable order
		// for results, so sort the port ranges.
		portRangesToUnits := ports.AllPortRanges()
		portRanges := make([]network.PortRange, 0, len(portRangesToUnits))
		for portRange := range portRangesToUnits {
			portRanges = append(portRanges, portRange)
		}
		network.SortPortRanges(portRanges)
		for _, portRange := range portRanges {
			unitName := portRangesToUnits[portRange]
			resultPorts = append(resultPorts, params.MachinePortRange{
				UnitTag:   names.NewUnitTag(unitName).String(),
				PortRange: portRange,
			})
		}
	}
	return params.MachinePortsResult{
		Ports: resultPorts,
	}
}
