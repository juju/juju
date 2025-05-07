// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/deployer"
)

type ManifoldsSuite struct {
	testing.IsolationSuite

	config deployer.UnitManifoldsConfig
}

var _ = tc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = deployer.UnitManifoldsConfig{
		Agent:         struct{ agent.Agent }{},
		LoggerContext: internallogger.LoggerContext(logger.DEBUG),
	}
}

func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	manifolds := deployer.UnitManifolds(s.config)
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	manifolds := deployer.UnitManifolds(s.config)
	expectedKeys := []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"s3-caller",
		"api-config-watcher",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"log-sender",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"uniter",
		"upgrader",
		"secret-drain-worker",
		"trace",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Assert(keys, tc.SameContents, expectedKeys)
}

func (s *ManifoldsSuite) TestMigrationGuards(c *tc.C) {
	exempt := set.NewStrings(
		"agent",
		"machine-lock",
		"api-config-watcher",
		"api-caller",
		"s3-caller",
		"log-sender",
		"upgrader",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"trace",
	)
	manifolds := deployer.UnitManifolds(s.config)
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
		deployer.UnitManifolds(s.config),
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

var expectedUnitManifoldsWithDependencies = map[string][]string{

	"agent": {},

	"api-address-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"charm-dir": {
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

	"leadership-tracker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},

	"log-sender": {"agent", "api-caller", "api-config-watcher"},

	"logging-config-updater": {
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
		"api-config-watcher"},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress"},

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
		"trace",
	},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
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
