// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
)

// Many manifolds completely depend on an agent and an API connection; this
// type configures them.
type AgentAPIManifoldConfig struct {
	AgentName     string
	APICallerName string
}

// AgentAPIStartFunc encapsulates the behaviour that varies among AgentAPIManifolds.
type AgentAPIStartFunc func(agent.Agent, base.APICaller) (worker.Worker, error)

// AgentAPIManifold returns a dependency.Manifold that calls the supplied start
// func with the API and agent resources defined in the config (once those
// resources are present).
func AgentAPIManifold(config AgentAPIManifoldConfig, start AgentAPIStartFunc) dependency.Manifold {
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
			return start(agent, apiCaller)
		},
	}
}
