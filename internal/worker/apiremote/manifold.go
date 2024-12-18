// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremote

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName      string
	CentralHubName string
	Clock          clock.Clock
	Logger         logger.Logger

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
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			// Get the agent.
			var agent coreagent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			agentConfig := agent.CurrentConfig()
			apiInfo, ready := agentConfig.APIInfo()
			if !ready {
				return nil, dependency.ErrMissing
			}

			var hub *pubsub.StructuredHub
			if err := getter.Get(config.CentralHubName, &hub); err != nil {
				return nil, err
			}

			cfg := WorkerConfig{
				Origin:    agentConfig.Tag(),
				Clock:     config.Clock,
				Hub:       hub,
				APIInfo:   apiInfo,
				NewRemote: NewRemoteServer,
				Logger:    config.Logger,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
		Output: remoteOutput,
	}
}

func remoteOutput(in worker.Worker, out interface{}) error {
	return nil
}
