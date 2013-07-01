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
	resources  common.ResourceRegistry
	authorizer common.Authorizer
}

// New creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(
	st *state.State,
	resources common.ResourceRegistry,
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
func (u *UpgraderAPI) WatchAPIVersion(args params.Agents) (params.EntityWatchResults, error) {
	result := params.EntityWatchResults{
		Results: make([]params.EntityWatchResult, len(args.Agents)),
	}
	for i, agent := range args.Agents {
		var err error
		if !u.authorizer.AuthOwner(agent.Tag) {
			err = common.ErrPerm
		} else {
			envWatcher := u.st.WatchEnvironConfig()
			result.Results[i].EntityWatcherId = u.resources.Register(envWatcher)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Find the Tools necessary for the given agents
func (u *UpgraderAPI) Tools(args params.Agents) (params.AgentToolsResults, error) {
	tools := make([]params.AgentToolsResult, len(args.Agents))
	result := params.AgentToolsResults{Tools: tools}
	if len(args.Agents) == 0 {
		return result, nil
	}
	for i, agent := range args.Agents {
		tools[i].Tag = agent.Tag
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
		var err error
		if !u.authorizer.AuthOwner(agent.Tag) {
			err = common.ErrPerm
		} else if agent.Arch == "" || agent.Series == "" {
			err = common.ErrBadRequest
		} else {
			requested := version.Binary{
				Number: agentVersion,
				Series: agent.Series,
				Arch:   agent.Arch,
			}
			// Note: (jam) We shouldn't have to search the provider
			//       for every machine that wants to upgrade. The
			//       information could just be cached in state, or
			//       even in the API servers
			tool, err := environs.FindExactTools(env, requested)
			if err == nil {
				// we have found tools for this agent
				tools[i].Arch = tool.Arch
				tools[i].Series = tool.Series
				tools[i].Major = tool.Major
				tools[i].Minor = tool.Minor
				tools[i].Patch = tool.Patch
				tools[i].Build = tool.Build
				tools[i].URL = tool.URL
			}
		}
		tools[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Find the Tools necessary for the given agents
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
