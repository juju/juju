// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/util"
)

// AgentApiManifoldTestConfig returns a AgentApiManifoldConfig
// suitable for use with RunAgentApiManifold.
func AgentApiManifoldTestConfig() util.AgentApiManifoldConfig {
	return util.AgentApiManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
	}
}

// RunAgentApiManifold is useful for testing manifolds based on
// AgentApiManifold. It takes the manifold, sets up the resources
// required to successfully pass AgentApiManifold's checks and then
// runs the manifold start func.
//
// An agent and apiCaller may be optionally provided. If they are nil,
// dummy barely-good-enough defaults will be used (these dummies are
// fine not actually used for much).
func RunAgentApiManifold(
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
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: agent},
		"api-caller-name": dt.StubResource{Output: apiCaller},
	})
	return manifold.Start(getResource)
}

type dummyAgent struct {
	agent.Agent
}
