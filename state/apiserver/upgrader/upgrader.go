// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"errors"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
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

func (u *UpgraderAPI) oneAgentTools(entity params.Entity, agentVersion version.Number, env environs.Environ) (*tools.Tools, error) {
	if !u.authorizer.AuthOwner(entity.Tag) {
		return nil, common.ErrPerm
	}
	agentEntity, err := u.st.AgentEntity(entity.Tag)
	if err != nil {
		return nil, err
	}

	existingTools, err := agentEntity.AgentTools()
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
	return environs.FindExactTools(env, requested)
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
		agentTools, err := u.oneAgentTools(entity, agentVersion, env)
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

func (u *UpgraderAPI) setOneAgentTools(tag string, tools *tools.Tools) error {
	if !u.authorizer.AuthOwner(tag) {
		return common.ErrPerm
	}
	// We assume that any entity that we can upgrade will
	// have a Life, which is certainly true now, but is
	// an assumption that may need revisiting at some point.
	entity, err := u.st.AgentEntity(tag)
	if err != nil {
		return err
	}
	return entity.SetAgentTools(tools)
}
