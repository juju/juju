// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: fakeAgent{},
	})
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{})
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	expectedKeys := []string{
		"agent",
		"termination",
		"api-caller",
		"api-info-gate",
	}
	c.Assert(expectedKeys, jc.SameContents, keys)
}

type fakeAgent struct {
	agent.Agent
}
