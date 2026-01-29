// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
}

// Manifold returns a dependency manifold that runs a machiner worker, using
// the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return newWorker(ctx, agent, apiCaller)
		},
	}
}

func newWorker(ctx context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	w, err := NewMachiner(Config{
		MachineAccessor: APIMachineAccessor{apimachiner.NewClient(apiCaller)},
		Tag:             a.CurrentConfig().Tag().(names.MachineTag),
	})
	return w, errors.Annotate(err, "starting machiner worker")
}
