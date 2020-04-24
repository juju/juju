// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	coreagent "github.com/juju/juju/agent"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName      string
	CentralHubName string
	Clock          clock.Clock
	Reporter       Reporter
	Logger         Logger

	NewWorker func(WorkerConfig) (worker.Worker, error)
}

// Manifold returns a dependency manifold that runs a pubsub
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.CentralHubName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Get the agent.
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			agentConfig := agent.CurrentConfig()

			// Get the hub.
			var hub *pubsub.StructuredHub
			if err := context.Get(config.CentralHubName, &hub); err != nil {
				return nil, err
			}

			apiInfo, ready := agentConfig.APIInfo()
			if !ready {
				return nil, dependency.ErrMissing
			}

			cfg := WorkerConfig{
				Origin:    agentConfig.Tag().String(),
				Clock:     config.Clock,
				Hub:       hub,
				APIInfo:   apiInfo,
				NewWriter: NewMessageWriter,
				NewRemote: NewRemoteServer,
				Logger:    config.Logger,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if r, ok := config.Reporter.(*reporter); ok {
				r.setWorker(w)
			}
			return w, nil
		},
	}
}
