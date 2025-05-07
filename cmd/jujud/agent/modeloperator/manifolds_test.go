// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeloperator_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/modeloperator"
	"github.com/juju/juju/internal/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ManifoldsSuite{})

type fakeAgent struct {
	agent.Agent
}

func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	manifolds := modeloperator.Manifolds(modeloperator.ManifoldConfig{
		Agent: fakeAgent{},
	})

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	manifolds := modeloperator.Manifolds(modeloperator.ManifoldConfig{Agent: &fakeAgent{}})
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}

	c.Check(keys, tc.SameContents, []string{
		"caas-broker-tracker",
		"api-caller",
		"log-sender",
		"caas-admission",
		"caas-rbac-mapper",
		"certificate-watcher",
		"logging-config-updater",
		"agent",
		"api-config-watcher",
		"upgrade-steps-gate",
		"model-http-server",
		"upgrader",
	})
}

func (s *ManifoldsSuite) TestManifoldsDependencies(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		modeloperator.Manifolds(modeloperator.ManifoldConfig{
			Agent: &fakeAgent{},
		}),
		expectedManifoldsWithDependencies,
	)
}

var expectedManifoldsWithDependencies = map[string][]string{

	"caas-broker-tracker": {"agent", "api-caller", "api-config-watcher"},

	"api-caller": {"agent", "api-config-watcher"},

	"log-sender": {"agent", "api-caller", "api-config-watcher"},

	"caas-admission": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-broker-tracker",
		"caas-rbac-mapper",
		"certificate-watcher",
		"model-http-server",
	},

	"caas-rbac-mapper": {"agent", "api-caller", "api-config-watcher", "caas-broker-tracker"},

	"certificate-watcher": {"agent"},

	"logging-config-updater": {"agent", "api-caller", "api-config-watcher"},

	"agent": {},

	"api-config-watcher": {"agent"},

	"upgrade-steps-gate": {},

	"model-http-server": {"agent", "certificate-watcher"},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
	},
}
