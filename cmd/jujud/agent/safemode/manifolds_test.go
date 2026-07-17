// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package safemode_test

import (
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/safemode"
	"github.com/juju/juju/internal/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

func TestManifoldsSuite(t *stdtesting.T) {
	tc.Run(t, &ManifoldsSuite{})
}

func (s *ManifoldsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	s.assertStartFuncs(c, safemode.Manifolds(safemode.ManifoldsConfig{}))
}

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	s.assertManifoldNames(c,
		safemode.Manifolds(safemode.ManifoldsConfig{}),
		[]string{
			"controller-agent-config",
			"db-accessor",
			"query-logger",
			"termination-signal-handler",
		},
	)
}

func (*ManifoldsSuite) assertManifoldNames(c *tc.C, manifolds dependency.Manifolds, expectedKeys []string) {
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Assert(keys, tc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestNoControllerFlagGuards(c *tc.C) {
	// The controller binary is always a controller node; no manifold
	// should reference the removed is-controller-flag or
	// state-config-watcher workers.
	manifolds := safemode.Manifolds(safemode.ManifoldsConfig{})

	for name, manifold := range manifolds {
		c.Logf("%s", name)
		checkNotContains(c, manifold.Inputs, "is-controller-flag")
		checkNotContains(c, manifold.Inputs, "state-config-watcher")
	}
}

func checkNotContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		c.Errorf("%q found in %v", seek, names)
		return
	}
}

func (s *ManifoldsSuite) TestManifoldsDependencies(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		safemode.Manifolds(safemode.ManifoldsConfig{}),
		expectedMachineManifoldsWithDependencies,
	)
}

var expectedMachineManifoldsWithDependencies = map[string][]string{
	"controller-agent-config": {},

	"db-accessor": {
		"controller-agent-config",
		"query-logger",
	},

	"query-logger": {},

	"termination-signal-handler": {},
}
