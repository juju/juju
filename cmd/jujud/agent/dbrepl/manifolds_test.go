// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl_test

import (
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/dbrepl"
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

func (s *ManifoldsSuite) TestStartFuncsIAAS(c *tc.C) {
	s.assertStartFuncs(c, dbrepl.IAASManifolds(newManifoldsConfig()))
}

func (s *ManifoldsSuite) TestStartFuncsCAAS(c *tc.C) {
	s.assertStartFuncs(c, dbrepl.CAASManifolds(newManifoldsConfig()))
}

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNamesIAAS(c *tc.C) {
	s.assertManifoldNames(c,
		dbrepl.IAASManifolds(newManifoldsConfig()),
		[]string{
			"controller-agent-config",
			"db-repl-accessor",
			"db-repl",
			"termination-signal-handler",
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *tc.C) {
	s.assertManifoldNames(c,
		dbrepl.CAASManifolds(newManifoldsConfig()),
		[]string{
			"controller-agent-config",
			"db-repl-accessor",
			"db-repl",
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
	manifolds := dbrepl.IAASManifolds(dbrepl.ManifoldsConfig{
		ControllerID:           "99",
		ConfigChangeSocketPath: "data-dir/configchange.socket",
		DataDir:                "data-dir",
		CACert:                 "ca-cert",
		ControllerCert:         "controller-cert",
		ControllerPrivateKey:   "controller-private-key",
	})

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

func (s *ManifoldsSuite) TestManifoldsDependenciesIAAS(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		dbrepl.IAASManifolds(newManifoldsConfig()),
		expectedMachineManifoldsWithDependenciesIAAS,
	)
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		dbrepl.CAASManifolds(newManifoldsConfig()),
		expectedMachineManifoldsWithDependenciesCAAS,
	)
}

var expectedMachineManifoldsWithDependenciesIAAS = map[string][]string{
	"controller-agent-config": {},

	"db-repl": {
		"db-repl-accessor",
	},

	"db-repl-accessor": {},

	"termination-signal-handler": {},
}

var expectedMachineManifoldsWithDependenciesCAAS = map[string][]string{
	"controller-agent-config": {},

	"db-repl": {
		"db-repl-accessor",
	},

	"db-repl-accessor": {},

	"termination-signal-handler": {},
}

func newManifoldsConfig() dbrepl.ManifoldsConfig {
	return dbrepl.ManifoldsConfig{
		ControllerID:           "99",
		ConfigChangeSocketPath: "data-dir/configchange.socket",
		DataDir:                "data-dir",
		CACert:                 "ca-cert",
		ControllerCert:         "controller-cert",
		ControllerPrivateKey:   "controller-private-key",
	}
}
