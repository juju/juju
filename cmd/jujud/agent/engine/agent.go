// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
)

// Some manifolds just depend on an agent; this type configures them.
type AgentManifoldConfig struct {
	AgentName string
}

// AgentStartFunc encapsulates the behaviour that varies among AgentManifolds.
type AgentStartFunc func(agent.Agent) (worker.Worker, error)

// AgentManifold returns a dependency.Manifold that calls the supplied start
// func with the agent resource defined in the config (once it's present).
func AgentManifold(config AgentManifoldConfig, start AgentStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			return start(agent)
		},
	}
}
