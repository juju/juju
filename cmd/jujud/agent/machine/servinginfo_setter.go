// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	coreagent "github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ServingInfoSetterConfig provides the dependencies for the
// servingInfoSetter manifold.
type ServingInfoSetterConfig struct {
	AgentName     string
	APICallerName string
}

// ServingInfoSetterManifold defines a simple start function which
// runs after the API connection has come up. If the machine agent is
// a controller, it grabs the state serving info over the API and
// records it to agent configuration, and then stops.
func ServingInfoSetterManifold(config ServingInfoSetterConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Get the agent.
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Grab the tag and ensure that it's for a machine.
			tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
			if !ok {
				return nil, errors.New("agent's tag is not a machine tag")
			}

			// Get API connection.
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			apiState := apiagent.NewState(apiCaller)

			// If the machine needs State, grab the state serving info
			// over the API and write it to the agent configuration.
			machine, err := apiState.Entity(tag)
			if err != nil {
				return nil, err
			}
			existing, hasInfo := agent.CurrentConfig().StateServingInfo()
			for _, job := range machine.Jobs() {
				if job.NeedsState() {
					info, err := apiState.StateServingInfo()
					if err != nil {
						return nil, errors.Errorf("cannot get state serving info: %v", err)
					}
					if hasInfo {
						// Use the existing Cert as it may have been updated already
						// by the cert updater worker to have this machine's IP address
						// as part of the cert. This changed cert is never put back into
						// the database, so it isn't reflected in the copy we have got
						// from apiState.
						info.Cert = existing.Cert
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
