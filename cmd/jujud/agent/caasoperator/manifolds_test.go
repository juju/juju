// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
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
		"charm-dir",
		"clock",
		"hook-retry-strategy",
		"operator",
		"logging-config-updater",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
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
	c.Assert(expectedKeys, jc.SameContents, keys)
}

type fakeAgent struct {
	agent.Agent
}
