// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	coreagent "github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/mongo"
	jworker "github.com/juju/juju/worker"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
}

// ManifoldConfig provides the dependencies for the
// agent config updater manifold.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        Logger
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
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Get the agent.
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Grab the tag and ensure that it's for a machine.
			currentConfig := agent.CurrentConfig()
			tag, ok := currentConfig.Tag().(names.MachineTag)
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

			controllerConfig, err := apiState.ControllerConfig()
			if err != nil {
				return nil, errors.Annotate(err, "getting controller config")
			}
			// If the mongo memory profile from the controller config
			// is different from the one in the agent config we need to
			// restart the agent to apply the memory profile to the mongo
			// service.
			logger := config.Logger
			mongoProfile := mongo.MemoryProfile(controllerConfig.MongoMemoryProfile())
			mongoProfileChanged := mongoProfile != currentConfig.MongoMemoryProfile()
			info, err := apiState.StateServingInfo()
			if err != nil {
				return nil, errors.Annotate(err, "getting state serving info")
			}
			err = agent.ChangeConfig(func(config coreagent.ConfigSetter) error {
				existing, hasInfo := config.StateServingInfo()
				if hasInfo {
					// Use the existing cert and key as they appear to
					// have been already updated by the cert updater
					// worker to have this machine's IP address as
					// part of the cert. This changed cert is never
					// put back into the database, so it isn't
					// reflected in the copy we have got from
					// apiState.
					info.Cert = existing.Cert
					info.PrivateKey = existing.PrivateKey
				}
				config.SetStateServingInfo(info)
				if mongoProfileChanged {
					logger.Debugf("setting agent config mongo memory profile: %s", mongoProfile)
					config.SetMongoMemoryProfile(mongoProfile)
				}
				return nil
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			// If we need a restart, return the fatal error.
			if mongoProfileChanged {
				logger.Infof("Restarting agent for new mongo memory profile")
				return nil, jworker.ErrRestartAgent
			}

			// All is well - we're done (no actual worker is actually returned).
			return nil, dependency.ErrUninstall
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
