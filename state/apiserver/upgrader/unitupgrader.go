// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
)

// UnitUpgraderAPI provides access to the UnitUpgrader API facade.
type UnitUpgraderAPI struct {
	*common.ToolsSetter

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	dataDir    string
}

// NewUnitUpgraderAPI creates a new server-side UnitUpgraderAPI facade.
func NewUnitUpgraderAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
	dataDir string,
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
		dataDir:     dataDir,
	}, nil
}

func (u *UnitUpgraderAPI) watchAssignedMachine(unitTag string) (string, error) {
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
	return "", watcher.MustErr(watch)
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade to. The watcher tracks changes to
// the unit's assigned machine since that's where the required agent version is stored.
func (u *UnitUpgraderAPI) WatchAPIVersion(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(agent.Tag) {
			var watcherId string
			watcherId, err = u.watchAssignedMachine(agent.Tag)
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
	if len(args.Entities) == 0 {
		return params.VersionResults{}, nil
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			result[i].Version, err = u.getMachineToolsVersion(entity.Tag)
		}
		result[i].Error = common.ServerError(err)
	}
	return params.VersionResults{result}, nil
}

// Tools finds the tools necessary for the given agents.
func (u *UnitUpgraderAPI) Tools(args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result.Results[i].Error = common.ServerError(common.ErrPerm)
		if u.authorizer.AuthOwner(entity.Tag) {
			result.Results[i] = u.getMachineTools(entity.Tag)
		}
	}
	return result, nil
}

func (u *UnitUpgraderAPI) getAssignedMachine(tag string) (*state.Machine, error) {
	// Check that we really have a unit tag.
	_, unitName, err := names.ParseTag(tag, names.UnitTagKind)
	if err != nil {
		return nil, common.ErrPerm
	}
	unit, err := u.st.Unit(unitName)
	if err != nil {
		return nil, common.ErrPerm
	}
	id, err := unit.AssignedMachineId()
	if err != nil {
		return nil, err
	}
	return u.st.Machine(id)
}

func (u *UnitUpgraderAPI) getMachineTools(tag string) params.ToolsResult {
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
	// For older 1.16 upgrader workers, we need to supply a tools URL since the worker will attempt to
	// download the tools even though they already have been fetched by the machine agent. Newer upgrader
	// workers do not have this problem. So to be compatible across all versions, we return the full
	// tools metadata.
	// TODO (wallyworld) - remove in 1.20, just return machineTools
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	// SSLHostnameVerification defaults to true, so we need to
	// invert that, for backwards-compatibility (older versions
	// will have DisableSSLHostnameVerification: false by default).
	result.DisableSSLHostnameVerification = !cfg.SSLHostnameVerification()
	env, err := environs.New(cfg)
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	agentTools, err := envtools.FindExactTools(
		env, machineTools.Version.Number, machineTools.Version.Series, machineTools.Version.Arch)
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	result.Tools = agentTools
	return result
}

func (u *UnitUpgraderAPI) getMachineToolsVersion(tag string) (*version.Number, error) {
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
