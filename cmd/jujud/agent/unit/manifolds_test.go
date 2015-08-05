// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/agent"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := unit.Manifolds(fakeAgent{}, nil)

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestAcyclic(c *gc.C) {
	manifolds := unit.Manifolds(fakeAgent{}, nil)
	count := len(manifolds)

	// Because we've already got outgoing links stored conveniently, we check
	// that the transpose of the graph is acyclic instead. Should be same.
	done := make(map[string]bool)
	doing := make(map[string]bool)
	sorted := make([]string, 0, count)

	// This _ hack to allow recursion feels less irritating that having to pull
	// out a whole type to embody this algroithm. Alternatives appreciated.
	visit := func(node string) {}
	visit_ := func(node string) {
		if doing[node] {
			c.Fatalf("cycle detected at %q", node)
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

	for node := range manifolds {
		visit(node)
	}
	c.Logf("sorted: %v", sorted)
	c.Check(sorted, gc.HasLen, count)
}

type fakeAgent struct {
	agent.Agent
}
