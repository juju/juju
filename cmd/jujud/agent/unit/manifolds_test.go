// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
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
	)
	config := unit.ManifoldsConfig{}
	manifolds := unit.Manifolds(config)
	for name, manifold := range manifolds {
		c.Logf(name)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
			checkContains(c, manifold.Inputs, "migration-fortress")
		}
	}
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
