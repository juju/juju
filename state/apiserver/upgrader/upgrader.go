// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"errors"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
)

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
	*common.ToolsGetter

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewUpgraderAPI creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*UpgraderAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &UpgraderAPI{
		ToolsGetter: common.NewToolsGetter(st, getCanRead),
		st:          st,
		resources:   resources,
		authorizer:  authorizer,
	}, nil
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(agent.Tag) {
			watch := u.st.WatchForEnvironConfigChanges()
			// Consume the initial event. Technically, API
			// calls to Watch 'transmit' the initial event
			// in the Watch response. But NotifyWatchers
			// have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result.Results[i].NotifyWatcherId = u.resources.Register(watch)
				err = nil
			} else {
				err = watcher.MustErr(watch)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UpgraderAPI) getGlobalAgentVersion() (version.Number, *config.Config, error) {
	// Get the Agent Version requested in the Environment Config
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return version.Number{}, nil, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return version.Number{}, nil, errors.New("agent version not set in environment config")
	}
	return agentVersion, cfg, nil
}

// DesiredVersion reports the Agent Version that we want that agent to be running
func (u *UpgraderAPI) DesiredVersion(args params.Entities) (params.VersionResults, error) {
	results := make([]params.VersionResult, len(args.Entities))
	if len(args.Entities) == 0 {
		return params.VersionResults{}, nil
	}
	agentVersion, _, err := u.getGlobalAgentVersion()
	if err != nil {
		return params.VersionResults{}, common.ServerError(err)
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			results[i].Version = &agentVersion
			err = nil
		}
		results[i].Error = common.ServerError(err)
	}
	return params.VersionResults{results}, nil
}

// SetTools updates the recorded tools version for the agents.
func (u *UpgraderAPI) SetTools(args params.EntitiesVersion) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.AgentTools)),
	}
	for i, agentTools := range args.AgentTools {
		err := u.setOneAgentVersion(agentTools.Tag, agentTools.Tools.Version)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (u *UpgraderAPI) setOneAgentVersion(tag string, vers version.Binary) error {
	if !u.authorizer.AuthOwner(tag) {
		return common.ErrPerm
	}
	entity, err := u.findEntity(tag)
	if err != nil {
		return err
	}
	return entity.SetAgentVersion(vers)
}

func (u *UpgraderAPI) findEntity(tag string) (state.AgentTooler, error) {
	entity0, err := u.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.AgentTooler)
	if !ok {
		return nil, common.NotSupportedError(tag, "agent tools")
	}
	return entity, nil
}
