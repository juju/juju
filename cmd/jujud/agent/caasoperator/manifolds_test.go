// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := caasoperator.Manifolds(caasoperator.ManifoldsConfig{
		Agent: fakeAgent{},
	})

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *gc.C) {
	config := caasoperator.ManifoldsConfig{}
	manifolds := caasoperator.Manifolds(config)
	expectedKeys := []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"clock",
		"hook-retry-strategy",
		"operator",
		"logging-config-updater",
		"log-sender",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"proxy-config-updater",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
		// TODO(caas)
		//"metric-spool",
		//"meter-status",
		//"metric-collect",
		//"metric-sender",
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
		"clock",
		"machine-lock",
		"api-config-watcher",
		"api-caller",
		"log-sender",
		"upgrader",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"upgrade-steps-gate",
		"upgrade-check-flag",
		"upgrade-steps-runner",
		"upgrade-steps-flag",
		"upgrade-check-gate",
	)
	config := caasoperator.ManifoldsConfig{}
	manifolds := caasoperator.Manifolds(config)
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
		caasoperator.Manifolds(caasoperator.ManifoldsConfig{
			Agent: fakeAgent{},
		}),
		expectedOperatorManifoldsWithDependencies,
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

var expectedOperatorManifoldsWithDependencies = map[string][]string{

	"agent": {},

	"api-address-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"charm-dir": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"hook-retry-strategy": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"log-sender": {"agent", "api-caller", "api-config-watcher"},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"migration-fortress": {
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"api-config-watcher"},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"proxy-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
	},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
	},

	"clock": {},

	"operator": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"clock",
		"hook-retry-strategy",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},
}
