// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
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
	// TODO: Unit agents are also allowed to use this API
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &UpgraderAPI{st: st, resources: resources, authorizer: authorizer}, nil
}

// Start a watcher to track if there is a new version of the API that we want
// to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Agents) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Agents)),
	}
	for i, agent := range args.Agents {
		var err error
		if !u.authorizer.AuthOwner(agent.Tag) {
			err = common.ErrPerm
		} else {
			envWatcher := u.st.WatchEnvironConfig()
			result.Results[i].NotifyWatcherId = u.resources.Register(envWatcher)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

var nilTools params.AgentTools

func (u *UpgraderAPI) oneAgentTools(agent params.Agent, agentVersion version.Number, env environs.Environ) (params.AgentTools, error) {
	if !u.authorizer.AuthOwner(agent.Tag) {
		return nilTools, common.ErrPerm
	}
	machine, err := u.st.Machine(state.MachineIdFromTag(agent.Tag))
	if err != nil {
		return nilTools, err
	}
	// TODO: Support Unit as well as Machine
	existingTools, err := machine.AgentTools()
	if err != nil {
		return nilTools, err
	}
	requested := version.Binary{
		Number: agentVersion,
		Series: existingTools.Series,
		Arch:   existingTools.Arch,
	}
	// Note: (jam) We shouldn't have to search the provider
	//       for every machine that wants to upgrade. The
	//       information could just be cached in state, or
	//       even in the API servers
	tools, err := environs.FindExactTools(env, requested)
	if err != nil {
		return nilTools, err
	}
	return params.AgentTools{
		Tag:    agent.Tag,
		Arch:   tools.Arch,
		Series: tools.Series,
		URL:    tools.URL,
		Major:  tools.Major,
		Minor:  tools.Minor,
		Patch:  tools.Patch,
		Build:  tools.Build,
	}, nil
}

// Find the Tools necessary for the given agents
func (u *UpgraderAPI) Tools(args params.Agents) (params.AgentToolsResults, error) {
	tools := make([]params.AgentToolsResult, len(args.Agents))
	result := params.AgentToolsResults{Tools: tools}
	if len(args.Agents) == 0 {
		return result, nil
	}
	for i, agent := range args.Agents {
		tools[i].AgentTools.Tag = agent.Tag
	}
	// For now, all agents get the same proposed version
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		// TODO: What error do we give here?
		return result, common.ErrBadRequest
	}
	env, err := environs.New(cfg)
	if err != nil {
		return result, err
	}
	for i, agent := range args.Agents {
		agentTools, err := u.oneAgentTools(agent, agentVersion, env)
		if err == nil {
			tools[i].AgentTools = agentTools
		}
		tools[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetTools updates the recorded tools version for the agents.
func (u *UpgraderAPI) SetTools(args params.SetAgentTools) (params.SetAgentToolsResults, error) {
	results := params.SetAgentToolsResults{
		Results: make([]params.SetAgentToolsResult, len(args.AgentTools)),
	}
	for i, tools := range args.AgentTools {
		var err error
		results.Results[i].Tag = tools.Tag
		if !u.authorizer.AuthOwner(tools.Tag) {
			err = common.ErrPerm
		} else {
			// TODO: When we get there, we should support setting
			//       Unit agent tools as well as Machine tools. We
			//       can use something like the "AgentState"
			//       interface that cmd/jujud/agent.go had.
			machine, err := u.st.Machine(state.MachineIdFromTag(tools.Tag))
			if err == nil {
				stTools := state.Tools{
					Binary: version.Binary{
						Number: version.Number{
							Major: tools.Major,
							Minor: tools.Minor,
							Patch: tools.Patch,
							Build: tools.Build,
						},
						Arch:   tools.Arch,
						Series: tools.Series,
					},
					URL: tools.URL,
				}
				err = machine.SetAgentTools(&stTools)
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}
