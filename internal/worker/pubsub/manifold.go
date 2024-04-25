// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

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
	Reporter       Reporter
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

			// Get the hub.
			var hub *pubsub.StructuredHub
			if err := getter.Get(config.CentralHubName, &hub); err != nil {
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
