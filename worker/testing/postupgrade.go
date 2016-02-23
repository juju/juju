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

// PostUpgradeManifoldTestConfig returns a PostUpgradeManifoldConfig
// suitable for use with RunPostUpgradeManifold.
func PostUpgradeManifoldTestConfig() util.PostUpgradeManifoldConfig {
	return util.PostUpgradeManifoldConfig{
		AgentName:         "agent-name",
		APICallerName:     "api-caller-name",
		UpgradeWaiterName: "upgradewaiter-name",
	}
}

// RunPostUpgradeManifold is useful for testing manifolds based on
// PostUpgradeManifold. It takes the manifold, sets up the resources
// required to successfully pass PostUpgradeManifold's checks and then
// runs the manifold start func.
//
// An agent and apiCaller may be optionally provided. If they are nil,
// dummy barely-good-enough default will be used (these dummies are
// fine not actually used for much).
func RunPostUpgradeManifold(
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
		"upgradewaiter-name": dt.StubResource{Output: true},
		"agent-name":         dt.StubResource{Output: agent},
		"api-caller-name":    dt.StubResource{Output: apiCaller},
	})
	return manifold.Start(getResource)
}

type dummyAgent struct {
	agent.Agent
}
