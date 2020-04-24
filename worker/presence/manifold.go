// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/presence"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName              string
	CentralHubName         string
	StateConfigWatcherName string
	Recorder               presence.Recorder
	Logger                 Logger

	NewWorker func(WorkerConfig) (worker.Worker, error)
}

// Validate ensures that the required values are set in the structure.
func (c *ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("missing AgentName")
	}
	if c.CentralHubName == "" {
		return errors.NotValidf("missing CentralHubName")
	}
	if c.StateConfigWatcherName == "" {
		return errors.NotValidf("missing StateConfigWatcherName")
	}
	if c.Recorder == nil {
		return errors.NotValidf("missing Recorder")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("missing NewWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a pubsub
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.CentralHubName,
			config.StateConfigWatcherName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, err
			}
			// Get the agent.
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				config.Logger.Tracef("agent dependency not available")
				return nil, err
			}
			origin := agent.CurrentConfig().Tag().String()

			// Get the hub.
			var hub *pubsub.StructuredHub
			if err := context.Get(config.CentralHubName, &hub); err != nil {
				config.Logger.Tracef("hub dependency not available")
				return nil, err
			}
			// Confirm we're running in a state server by asking the
			// stateconfigwatcher manifold.
			var haveStateConfig bool
			if err := context.Get(config.StateConfigWatcherName, &haveStateConfig); err != nil {
				config.Logger.Tracef("state config watcher not available")
				return nil, err
			}
			if !haveStateConfig {
				config.Logger.Tracef("not a state server, not needed")
				config.Recorder.Disable()
				return nil, dependency.ErrMissing
			}
			config.Recorder.Enable()

			cfg := WorkerConfig{
				Origin:   origin,
				Hub:      hub,
				Recorder: config.Recorder,
				Logger:   config.Logger,
			}
			return config.NewWorker(cfg)
		},
	}
}
