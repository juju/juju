// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
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

// TODO(cmars) 2015/08/10: rework this into builtin Engine cycle checker.
func (s *ManifoldsSuite) TestAcyclic(c *gc.C) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})
	count := len(manifolds)

	// Set of vars for depth-first topological sort of manifolds. (Note that,
	// because we've already got outgoing links stored conveniently, we're
	// actually checking the transpose of the dependency graph. Cycles will
	// still be cycles in either direction, though.)
	done := make(map[string]bool)
	doing := make(map[string]bool)
	sorted := make([]string, 0, count)

	// Stupid _-suffix malarkey allows recursion. Seems cleaner to keep these
	// considerations inside this func than to embody the algorithm in a type.
	visit := func(node string) {}
	visit_ := func(node string) {
		if doing[node] {
			c.Fatalf("cycle detected at %q (considering: %v)", node, doing)
		}
		if !done[node] {
			doing[node] = true
			for _, input := range manifolds[node].Inputs {
				visit(input)
			}
			done[node] = true
			doing[node] = false
			sorted = append(sorted, node)
		}
	}
	visit = visit_

	// Actually sort them, or fail if we find a cycle.
	for node := range manifolds {
		visit(node)
	}
	c.Logf("got: %v", sorted)
	c.Check(sorted, gc.HasLen, count) // Final sanity check.
}

type fakeAgent struct {
	agent.Agent
}
