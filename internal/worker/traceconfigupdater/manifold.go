// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceconfigupdater

import (
	"context"

	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/tracer"
	"github.com/juju/juju/api/base"
	corelogger "github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName          string
	APICallerName      string
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
}

// Manifold returns a dependency manifold that runs a trace config updater
// worker using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			return NewWorker(WorkerConfig{
				Agent:              a,
				API:                tracer.NewClient(apiCaller),
				AgentConfigChanged: config.AgentConfigChanged,
				Logger:             config.Logger,
			})
		},
	}
}
