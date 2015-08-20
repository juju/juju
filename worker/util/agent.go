// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
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
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			return start(agent)
		},
	}
}
