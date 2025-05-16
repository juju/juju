// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/containeragent/unit"
	"github.com/juju/juju/internal/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

func TestManifoldsSuite(t *stdtesting.T) { tc.Run(t, &ManifoldsSuite{}) }
func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	config := unit.ManifoldsConfig{}
	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"api-config-watcher",
		"caas-prober",
		"caas-unit-termination-worker",
		"caas-unit-prober-binder",
		"caas-zombie-prober-binder",
		"charm-dir",
		"dead-flag",
		"hook-retry-strategy",
		"leadership-tracker",
		"log-sender",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"not-dead-flag",
		"probe-http-server",
		"proxy-config-updater",
		"s3-caller",
		"secret-drain-worker",
		"signal-handler",

		"trace",
		"uniter",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(keys, tc.SameContents, expectedKeys)
}

func (s *ManifoldsSuite) TestManifoldNamesColocatedController(c *tc.C) {
	config := unit.ManifoldsConfig{
		ColocatedWithController: true,
	}
	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-prober",
		"caas-unit-prober-binder",
		"caas-unit-termination-worker",
		"caas-zombie-prober-binder",
		"charm-dir",
		"dead-flag",
		"hook-retry-strategy",
		"leadership-tracker",
		"log-sender",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"not-dead-flag",
		"probe-http-server",
		"proxy-config-updater",
		"s3-caller",
		"secret-drain-worker",
		"signal-handler",

		"trace",
		"uniter",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(keys, tc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestMigrationGuards(c *tc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-config-watcher",
		"api-caller",
		"s3-caller",
		"caas-prober",
		"probe-http-server",
		"caas-unit-prober-binder",
		"log-sender",

		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",

		"upgrader",
		"upgrade-steps-runner",
		"upgrade-steps-gate",

		"upgrade-steps-flag",

		"dead-flag",
		"not-dead-flag",
		"signal-handler",
		"caas-zombie-prober",
		"caas-zombie-prober-binder",

		"trace",
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

func (s *ManifoldsSuite) TestManifoldsDependencies(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		unit.Manifolds(unit.ManifoldsConfig{
			Agent: fakeAgent{},
		}),
		expectedUnitManifoldsWithDependencies,
	)
}

func checkContains(c *tc.C, names []string, seek string) {
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
		"trace",
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
		"probe-http-server",
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
		"trace",
	},
	"caas-unit-prober-binder": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-prober",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"probe-http-server",
		"s3-caller",
		"trace",
		"uniter",
	},
	"caas-zombie-prober-binder": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-prober",
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
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
	},
	"trace": {
		"agent",
	},
}
