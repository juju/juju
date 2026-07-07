// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl_test

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/cmd/jujuagentd/agent/dbrepl"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/gate"
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

func (s *ManifoldsSuite) TestManifoldsDependenciesIAAS(c *tc.C) {
	manifolds := dbrepl.IAASManifolds(newManifoldsConfig())
	for name, expected := range expectedManifoldsWithDependencies {
		manifold, ok := manifolds[name]
		c.Assert(ok, tc.IsTrue, tc.Commentf("manifold %q not found", name))
		c.Check(manifold.Inputs, tc.SameContents, expected, tc.Commentf("manifold %q", name))
	}
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *tc.C) {
	manifolds := dbrepl.CAASManifolds(newManifoldsConfig())
	for name, expected := range expectedManifoldsWithDependencies {
		manifold, ok := manifolds[name]
		c.Assert(ok, tc.IsTrue, tc.Commentf("manifold %q not found", name))
		c.Check(manifold.Inputs, tc.SameContents, expected, tc.Commentf("manifold %q", name))
	}
}

var expectedManifoldsWithDependencies = map[string][]string{
	"controller-agent-config": {},

	"db-repl": {"db-repl-accessor"},

	"db-repl-accessor": {},

	"termination-signal-handler": {},
}

func newManifoldsConfig() dbrepl.ManifoldsConfig {
	unlocker := gate.NewLock()
	unlocker.Unlock()
	return dbrepl.ManifoldsConfig{
		ControllerUnlocker:     unlocker,
		ControllerID:           testing.ControllerTag.Id(),
		ConfigChangeSocketPath: "data-dir/configchange.socket",
		DataDir:                "data-dir",
		CACert:                 "ca-cert",
		ControllerCert:         "controller-cert",
		ControllerPrivateKey:   "controller-private-key",
	}
}
