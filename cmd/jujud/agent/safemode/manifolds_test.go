// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package safemode_test

import (
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/safemode"
	"github.com/juju/juju/controller"
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
	s.assertStartFuncs(c, safemode.IAASManifolds(safemode.ManifoldsConfig{}))
}

func (s *ManifoldsSuite) TestStartFuncsCAAS(c *tc.C) {
	s.assertStartFuncs(c, safemode.CAASManifolds(safemode.ManifoldsConfig{}))
}

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNamesIAAS(c *tc.C) {
	s.assertManifoldNames(c,
		safemode.IAASManifolds(safemode.ManifoldsConfig{}),
		[]string{
			"controller-agent-config",
			"db-accessor",
			"query-logger",
			"termination-signal-handler",
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *tc.C) {
	s.assertManifoldNames(c,
		safemode.CAASManifolds(safemode.ManifoldsConfig{}),
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
	manifolds := safemode.IAASManifolds(safemode.ManifoldsConfig{})

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
		safemode.IAASManifolds(safemode.ManifoldsConfig{}),
		expectedMachineManifoldsWithDependenciesIAAS,
	)
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		safemode.CAASManifolds(safemode.ManifoldsConfig{}),
		expectedMachineManifoldsWithDependenciesCAAS,
	)
}

var expectedMachineManifoldsWithDependenciesIAAS = map[string][]string{

	"controller-agent-config": {},

	"db-accessor": {
		"controller-agent-config",
		"query-logger",
	},

	"query-logger": {},

	"termination-signal-handler": {},
}

var expectedMachineManifoldsWithDependenciesCAAS = map[string][]string{

	"controller-agent-config": {},

	"db-accessor": {
		"controller-agent-config",
		"query-logger",
	},

	"query-logger": {},

	"termination-signal-handler": {},
}

type mockAgent struct {
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag      names.Tag
	ssiSet   bool
	ssi      controller.ControllerAgentInfo
	dataPath string
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.ControllerAgentInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.ControllerAgentInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}

func (mc *mockConfig) DataDir() string {
	if mc.dataPath != "" {
		return mc.dataPath
	}
	return "data-dir"
}
