// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
)

// UnitUpgraderAPI provides access to the UnitUpgrader API facade.
type UnitUpgraderAPI struct {
	*common.ToolsSetter

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewUnitUpgraderAPI creates a new server-side UnitUpgraderAPI facade.
func NewUnitUpgraderAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*UnitUpgraderAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}

	getCanWrite := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &UnitUpgraderAPI{
		ToolsSetter: common.NewToolsSetter(st, getCanWrite),
		st:          st,
		resources:   resources,
		authorizer:  authorizer,
	}, nil
}

func (u *UnitUpgraderAPI) watchAssignedMachine(unitTag names.Tag) (string, error) {
	machine, err := u.getAssignedMachine(unitTag)
	if err != nil {
		return "", err
	}
	watch := machine.Watch()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade to. The watcher tracks changes to
// the unit's assigned machine since that's where the required agent version is stored.
func (u *UnitUpgraderAPI) WatchAPIVersion(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		tag, err := names.ParseTag(agent.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			var watcherId string
			watcherId, err = u.watchAssignedMachine(tag)
			if err == nil {
				result.Results[i].NotifyWatcherId = watcherId
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// DesiredVersion reports the Agent Version that we want that unit to be running.
// The desired version is what the unit's assigned machine is running.
func (u *UnitUpgraderAPI) DesiredVersion(args params.Entities) (params.VersionResults, error) {
	result := make([]params.VersionResult, len(args.Entities))
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			result[i].Version, err = u.getMachineToolsVersion(tag)
		}
		result[i].Error = common.ServerError(err)
	}
	return params.VersionResults{Results: result}, nil
}

// Tools finds the tools necessary for the given agents.
func (u *UnitUpgraderAPI) Tools(args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result.Results[i].Error = common.ServerError(common.ErrPerm)
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			continue
		}
		if u.authorizer.AuthOwner(tag) {
			result.Results[i] = u.getMachineTools(tag)
		}
	}
	return result, nil
}

func (u *UnitUpgraderAPI) getAssignedMachine(tag names.Tag) (*state.Machine, error) {
	// Check that we really have a unit tag.
	switch tag := tag.(type) {
	case names.UnitTag:
		unit, err := u.st.Unit(tag.Id())
		if err != nil {
			return nil, common.ErrPerm
		}
		id, err := unit.AssignedMachineId()
		if err != nil {
			return nil, err
		}
		return u.st.Machine(id)
	default:
		return nil, common.ErrPerm
	}
}

func (u *UnitUpgraderAPI) getMachineTools(tag names.Tag) params.ToolsResult {
	var result params.ToolsResult
	machine, err := u.getAssignedMachine(tag)
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	machineTools, err := machine.AgentTools()
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	result.Tools = machineTools
	return result
}

func (u *UnitUpgraderAPI) getMachineToolsVersion(tag names.Tag) (*version.Number, error) {
	machine, err := u.getAssignedMachine(tag)
	if err != nil {
		return nil, err
	}
	machineTools, err := machine.AgentTools()
	if err != nil {
		return nil, err
	}
	return &machineTools.Version.Number, nil
}
