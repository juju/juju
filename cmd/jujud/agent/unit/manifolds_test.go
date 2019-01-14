// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/unit"
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
		"log-sender",
		"upgrader",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"logging-config-updater",
		"proxy-config-updater",
		"api-address-updater",
		"charm-dir",
		"leadership-tracker",
		"hook-retry-strategy",
		"uniter",
		"metric-spool",
		"meter-status",
		"metric-collect",
		"metric-sender",
		"upgrade-steps-flag",
		"upgrade-steps-runner",
		"upgrade-steps-gate",
		"upgrade-check-gate",
		"upgrade-check-flag",
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(expectedKeys, jc.SameContents, keys)
}

func (*ManifoldsSuite) TestMigrationGuards(c *gc.C) {
	exempt := set.NewStrings(
		"agent",
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

	"api-address-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
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
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"hook-retry-strategy": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"leadership-tracker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"log-sender": {"agent", "api-caller", "api-config-watcher"},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"meter-status": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"metric-collect": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"metric-spool",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"metric-sender": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"metric-spool",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"metric-spool": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"migration-fortress": {
		"upgrade-check-flag",
		"upgrade-check-gate",
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
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"proxy-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"uniter": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate"},

	"upgrade-check-flag": {"upgrade-check-gate"},

	"upgrade-check-gate": {},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate"},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-check-gate",
		"upgrade-steps-gate"},
}
