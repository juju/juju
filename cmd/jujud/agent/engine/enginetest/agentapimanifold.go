// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package enginetest

import (
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// AgentAPIManifoldTestConfig returns a AgentAPIManifoldConfig
// suitable for use with RunAgentAPIManifold.
func AgentAPIManifoldTestConfig() engine.AgentAPIManifoldConfig {
	return engine.AgentAPIManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
	}
}

// RunAgentAPIManifold is useful for testing manifolds based on
// AgentAPIManifold. It takes the manifold, sets up the resources
// required to successfully pass AgentAPIManifold's checks and then
// runs the manifold start func.
//
// An agent and apiCaller may be optionally provided. If they are nil,
// dummy barely-good-enough defaults will be used (these dummies are
// fine not actually used for much).
func RunAgentAPIManifold(
	manifold dependency.Manifold, agent agent.Agent, apiCaller base.APICaller,
) (worker.Worker, error) {
	if agent == nil {
		agent = new(dummyAgent)
	}
	if apiCaller == nil {
		apiCaller = basetesting.APICallerFunc(
			func(string, int, string, string, interface{}, interface{}) error {
				return nil
			})
	}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      agent,
		"api-caller-name": apiCaller,
	})
	return manifold.Start(context)
}

type dummyAgent struct {
	agent.Agent
}
