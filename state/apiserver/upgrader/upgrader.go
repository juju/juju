// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"errors"

	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	agenttools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
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
	return &UpgraderAPI{st: st, resources: resources, authorizer: authorizer}, nil
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

func (u *UpgraderAPI) oneAgentTools(tag string, agentVersion version.Number, env environs.Environ) (*agenttools.Tools, error) {
	if !u.authorizer.AuthOwner(tag) {
		return nil, common.ErrPerm
	}
	entity0, err := u.findEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.AgentTooler)
	if !ok {
		return nil, common.NotSupportedError(tag, "agent tools")
	}

	existingTools, err := entity.AgentTools()
	if err != nil {
		return nil, err
	}
	requested := version.Binary{
		Number: agentVersion,
		Series: existingTools.Version.Series,
		Arch:   existingTools.Version.Arch,
	}
	// TODO(jam): Avoid searching the provider for every machine
	// that wants to upgrade. The information could just be cached
	// in state, or even in the API servers
	return envtools.FindExactTools(env, requested)
}

// DesiredVersion reports the Agent Version that we want that agent to be running
func (u *UpgraderAPI) DesiredVersion(args params.Entities) (params.AgentVersionResults, error) {
	results := make([]params.AgentVersionResult, len(args.Entities))
	if len(args.Entities) == 0 {
		return params.AgentVersionResults{}, nil
	}
	// For now, all agents get the same proposed version
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return params.AgentVersionResults{}, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return params.AgentVersionResults{}, errors.New("agent version not set in environment config")
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			results[i].Version = &agentVersion
			err = nil
		}
		results[i].Error = common.ServerError(err)
	}
	return params.AgentVersionResults{results}, nil
}

// Tools finds the Tools necessary for the given agents.
func (u *UpgraderAPI) Tools(args params.Entities) (params.AgentToolsResults, error) {
	results := make([]params.AgentToolsResult, len(args.Entities))
	if len(args.Entities) == 0 {
		return params.AgentToolsResults{}, nil
	}
	// For now, all agents get the same proposed version
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return params.AgentToolsResults{}, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return params.AgentToolsResults{}, errors.New("agent version not set in environment config")
	}
	env, err := environs.New(cfg)
	if err != nil {
		return params.AgentToolsResults{}, err
	}
	for i, entity := range args.Entities {
		agentTools, err := u.oneAgentTools(entity.Tag, agentVersion, env)
		if err == nil {
			results[i].Tools = agentTools
		}
		results[i].Error = common.ServerError(err)
	}
	return params.AgentToolsResults{results}, nil
}

// SetTools updates the recorded tools version for the agents.
func (u *UpgraderAPI) SetTools(args params.SetAgentsTools) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.AgentTools)),
	}
	for i, agentTools := range args.AgentTools {
		err := u.setOneAgentTools(agentTools.Tag, agentTools.Tools)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (u *UpgraderAPI) setOneAgentTools(tag string, tools *agenttools.Tools) error {
	if !u.authorizer.AuthOwner(tag) {
		return common.ErrPerm
	}
	entity, err := u.findEntity(tag)
	if err != nil {
		return err
	}
	return entity.SetAgentTools(tools)
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
