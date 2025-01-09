// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/common"
)

// APIRemoteCallers is an interface that represents the remote API callers.
type APIRemoteCallers interface {
	// GetAPIRemotes returns the current API connections. It is expected that
	// the caller will call this method just before making an API call to ensure
	// that the connection is still valid. The caller must not cache the
	// connections as they may change over time.
	GetAPIRemotes() []RemoteConnection
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName      string
	CentralHubName string
	Clock          clock.Clock
	Logger         logger.Logger

	NewWorker func(WorkerConfig) (worker.Worker, error)
}

// Manifold returns a dependency manifold that runs a API remote caller worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.CentralHubName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
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
				Hub:       hub,
				APIInfo:   apiInfo,
				Origin:    agentConfig.Tag(),
				NewRemote: NewRemoteServer,
				Logger:    config.Logger,
				Clock:     config.Clock,
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
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*remoteWorker)
	if !ok {
		return errors.Errorf("expected input of type remoteWorker, got %T", in)
	}

	switch out := out.(type) {
	case *APIRemoteCallers:
		var target APIRemoteCallers = w
		*out = target
	default:
		return errors.Errorf("expected output of APIRemoteCallers, got %T", out)
	}
	return nil
}
