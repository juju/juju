// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/containeragent/unit"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *gc.C) {
	config := unit.ManifoldsConfig{}
	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		"agent",
		"api-config-watcher",
		"api-caller",
		"s3-caller",
		"uniter",
		"log-sender",

		"charm-dir",
		"leadership-tracker",
		"hook-retry-strategy",

		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",

		"proxy-config-updater",
		"logging-config-updater",
		"api-address-updater",

		"caas-prober",
		"probe-http-server",

		"upgrader",
		"upgrade-steps-runner",
		"upgrade-steps-gate",
		"upgrade-steps-flag",

		"caas-unit-termination-worker",
		"caas-units-manager",
		"secret-drain-worker",

		"caas-zombie-prober",

		"dead-flag",
		"not-dead-flag",

		"signal-handler",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(keys, jc.SameContents, expectedKeys)
}

func (s *ManifoldsSuite) TestManifoldNamesColocatedController(c *gc.C) {
	config := unit.ManifoldsConfig{
		ColocatedWithController: true,
	}
	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		"agent",
		"api-config-watcher",
		"api-caller",
		"s3-caller",
		"caas-prober",
		"probe-http-server",
		"uniter",
		"log-sender",

		"charm-dir",
		"leadership-tracker",
		"hook-retry-strategy",

		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",

		"proxy-config-updater",
		"logging-config-updater",

		"upgrader",
		"upgrade-steps-runner",
		"upgrade-steps-gate",
		"upgrade-steps-flag",

		"caas-unit-termination-worker",
		"caas-units-manager",
		"controller-charm",
		"secret-drain-worker",
		"caas-zombie-prober",

		"dead-flag",
		"not-dead-flag",

		"signal-handler",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(keys, jc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestMigrationGuards(c *gc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-config-watcher",
		"api-caller",
		"s3-caller",
		"caas-prober",
		"probe-http-server",
		"log-sender",

		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",

		"upgrader",
		"upgrade-steps-runner",
		"upgrade-steps-gate",

		"upgrade-steps-flag",
		"caas-units-manager",

		"dead-flag",
		"not-dead-flag",
		"signal-handler",
		"caas-zombie-prober",
	)
	config := unit.ManifoldsConfig{}
	manifolds := unit.Manifolds(config)
	for name, manifold := range manifolds {
		c.Logf("%v [%v]", name, manifold.Inputs)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
			checkContains(c, manifold.Inputs, "migration-fortress")
		}
	}
}

func (s *ManifoldsSuite) TestManifoldsDependencies(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		unit.Manifolds(unit.ManifoldsConfig{
			Agent: fakeAgent{},
		}),
		expectedUnitManifoldsWithDependencies,
	)
}

func checkContains(c *gc.C, names []string, seek string) {
	for _, name := range names {
		if name == seek {
			return
		}
	}
	c.Errorf("%q not present in %v", seek, names)
}

type fakeAgent struct {
	agent.Agent
}

var expectedUnitManifoldsWithDependencies = map[string][]string{

	"agent": {},
	"api-config-watcher": {
		"agent",
	},
	"api-caller": {
		"agent",
		"api-config-watcher",
	},
	"s3-caller": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},
	"uniter": {
		"agent",
		"api-caller",
		"s3-caller",
		"api-config-watcher",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"log-sender": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"not-dead-flag",
	},

	"charm-dir": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"leadership-tracker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"hook-retry-strategy": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},

	"migration-fortress": {},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
	},

	"proxy-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"logging-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"api-address-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"probe-http-server": {},
	"caas-prober": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"probe-http-server",
		"s3-caller",
		"uniter",
		"not-dead-flag",
	},
	"upgrade-steps-flag": {
		"upgrade-steps-gate",
	},
	"upgrade-steps-gate": {},
	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
		"not-dead-flag",
	},
	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
		"not-dead-flag",
	},

	"caas-unit-termination-worker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"s3-caller",
		"uniter",
		"not-dead-flag",
	},
	"caas-units-manager": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"not-dead-flag",
	},
	"caas-zombie-prober": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"dead-flag",
		"probe-http-server",
	},

	"dead-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},
	"not-dead-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},

	"signal-handler": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"dead-flag",
	},
	"secret-drain-worker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},
}
