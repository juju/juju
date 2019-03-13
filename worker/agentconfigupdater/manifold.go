// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	coreagent "github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Criticalf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// ManifoldConfig provides the dependencies for the
// agent config updater manifold.
type ManifoldConfig struct {
	AgentName      string
	APICallerName  string
	CentralHubName string
	Logger         Logger
}

// Manifold defines a simple start function which
// runs after the API connection has come up. If the machine agent is
// a controller, it grabs the state serving info over the API and
// records it to agent configuration, and then stops.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.CentralHubName,
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
			apiState, err := apiagent.NewState(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// If the machine needs State, grab the state serving info
			// over the API and write it to the agent configuration.
			if controller, err := isController(apiState, tag); err != nil {
				return nil, errors.Annotate(err, "checking controller status")
			} else if !controller {
				// Not a controller, nothing to do.
				return nil, dependency.ErrUninstall
			}

			// Only get the hub if we are a controller.
			var hub *pubsub.StructuredHub
			if err := context.Get(config.CentralHubName, &hub); err != nil {
				config.Logger.Tracef("hub dependency not available")
				return nil, err
			}

			info, err := apiState.StateServingInfo()

			return NewWorker(WorkerConfig{
				Agent:        agent,
				ConfigGetter: apiState,
				Hub:          hub,
				Logger:       config.Logger,
			})
		},
	}
}

func isController(apiState *apiagent.State, tag names.MachineTag) (bool, error) {
	machine, err := apiState.Entity(tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, job := range machine.Jobs() {
		if job.NeedsState() {
			return true, nil
		}
	}
	return false, nil
}
