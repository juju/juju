// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// servingInfoSetterConfig provides the dependencies for the
// servingInfoSetter manifold.
type servingInfoSetterConfig struct {
	AgentName     string
	APICallerName string
}

// servingInfoSetterManifold defines a simple start function which
// runs after the API connection has come up. If the machine agent is
// a state server, it grabs the state serving info over the API and
// records it to agent configuration, and then stops.
func servingInfoSetterManifold(config servingInfoSetterConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			// Get the agent.
			var agent coreagent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Grab the tag and ensure that it's for a machine.
			tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
			if !ok {
				return nil, errors.New("agent's tag is not a machine tag")
			}

			// Get API connection.
			//
			// TODO(mjs) - this should really be a base.APICaller to
			// remove the possibility of the API connection being closed
			// here.
			var apiConn api.Connection
			if err := getResource(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}

			// If the machine needs State, grab the state serving info
			// over the API and write it to the agent configuration.
			//
			// TODO(mjs) - ideally this would be using its own facade.
			machine, err := apiConn.Agent().Entity(tag)
			if err != nil {
				return nil, err
			}
			for _, job := range machine.Jobs() {
				if job.NeedsState() {
					info, err := apiConn.Agent().StateServingInfo()
					if err != nil {
						return nil, errors.Errorf("cannot get state serving info: %v", err)
					}
					err = agent.ChangeConfig(func(config coreagent.ConfigSetter) error {
						config.SetStateServingInfo(info)
						return nil
					})
					if err != nil {
						return nil, err
					}
				}
			}

			// All is well - we're done (no actual worker is actually returned).
			return nil, dependency.ErrUninstall
		},
	}
}
