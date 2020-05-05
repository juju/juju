// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	coreagent "github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/mongo"
	jworker "github.com/juju/juju/worker"
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

			// Grab the tag and ensure that it's for a controller.
			tag := agent.CurrentConfig().Tag()
			if !apiagent.IsAllowedControllerTag(tag.Kind()) {
				return nil, errors.New("agent's tag is not a machine or controller agent tag")
			}

			// Get API connection.
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			// If the machine needs State, grab the state serving info
			// over the API and write it to the agent configuration.
			if controller, err := apiagent.IsController(apiCaller, tag); err != nil {
				return nil, errors.Annotate(err, "checking controller status")
			} else if !controller {
				// Not a controller, nothing to do.
				return nil, dependency.ErrUninstall
			}

			// Do the initial state serving info and mongo profile checks
			// before attempting to get the central hub. The central hub is only
			// running when the agent is a controller. If the agent isn't a controller
			// but should be, the agent config will not have any state serving info
			// but the database will think that we should be. In those situations
			// we need to update the local config and restart.
			apiState, err := apiagent.NewState(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
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
			agentsMongoMemoryProfile := agent.CurrentConfig().MongoMemoryProfile()
			configMongoMemoryProfile := mongo.MemoryProfile(controllerConfig.MongoMemoryProfile())
			mongoProfileChanged := agentsMongoMemoryProfile != configMongoMemoryProfile

			agentsJujuDBSnapChannel := agent.CurrentConfig().JujuDBSnapChannel()
			configJujuDBSnapChannel := controllerConfig.JujuDBSnapChannel()
			jujuDBSnapChannelChanged := agentsJujuDBSnapChannel != configJujuDBSnapChannel

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
					logger.Debugf("setting agent config mongo memory profile: %q => %q", agentsMongoMemoryProfile, configMongoMemoryProfile)
					config.SetMongoMemoryProfile(configMongoMemoryProfile)
				}
				if jujuDBSnapChannelChanged {
					logger.Debugf("setting agent config mongo snap channel: %q => %q", agentsJujuDBSnapChannel, configJujuDBSnapChannel)
					config.SetJujuDBSnapChannel(configJujuDBSnapChannel)
				}
				return nil
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			// If we need a restart, return the fatal error.
			if mongoProfileChanged {
				logger.Infof("restarting agent for new mongo memory profile")
				return nil, jworker.ErrRestartAgent
			} else if jujuDBSnapChannelChanged {
				logger.Infof("restarting agent for new mongo snap channel")
				return nil, jworker.ErrRestartAgent
			}

			// Only get the hub if we are a controller and we haven't updated
			// the memory profile.
			var hub *pubsub.StructuredHub
			if err := context.Get(config.CentralHubName, &hub); err != nil {
				logger.Tracef("hub dependency not available")
				return nil, err
			}

			return NewWorker(WorkerConfig{
				Agent:             agent,
				Hub:               hub,
				MongoProfile:      configMongoMemoryProfile,
				JujuDBSnapChannel: configJujuDBSnapChannel,
				Logger:            config.Logger,
			})
		},
	}
}
