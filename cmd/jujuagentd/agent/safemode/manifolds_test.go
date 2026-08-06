// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package safemode_test

import (
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/jujuagentd/agent/safemode"
	"github.com/juju/juju/controller"
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

func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	s.assertStartFuncs(c, safemode.Manifolds(safemode.ManifoldsConfig{
		Agent:              &mockAgent{},
		ControllerUnlocker: gate.NewLock(),
	}))
}

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	s.assertManifoldNames(c,
		safemode.Manifolds(safemode.ManifoldsConfig{
			Agent:              &mockAgent{},
			ControllerUnlocker: gate.NewLock(),
		}),
		[]string{
			"agent",
			"controller-agent-config",
			"db-accessor",
			"is-controller-flag",
			"query-logger",
			"state-config-watcher",
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

func (*ManifoldsSuite) TestSingularGuardsUsed(c *tc.C) {
	manifolds := safemode.Manifolds(safemode.ManifoldsConfig{
		Agent:              &mockAgent{},
		ControllerUnlocker: gate.NewLock(),
	})

	// Explicitly guarded by ifController.
	controllerWorkers := set.NewStrings(
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
	)

	// Explicitly guarded by ifPrimaryController.
	primaryControllerWorkers := set.NewStrings()

	dbUpgradedWorkers := set.NewStrings()

	for name, manifold := range manifolds {
		c.Logf("%s", name)
		switch {
		case controllerWorkers.Contains(name):
			checkContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
		case primaryControllerWorkers.Contains(name):
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkContains(c, manifold.Inputs, "is-primary-controller-flag")
		case dbUpgradedWorkers.Contains(name):
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
			checkContains(c, manifold.Inputs, "upgrade-database-flag")
		default:
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
		}
	}
}

func checkContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		return
	}
	c.Errorf("%q not found in %v", seek, names)
}

func checkNotContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		c.Errorf("%q found in %v", seek, names)
		return
	}
}

func (s *ManifoldsSuite) TestManifoldsDependencies(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		safemode.Manifolds(safemode.ManifoldsConfig{
			Agent:              &mockAgent{},
			ControllerUnlocker: gate.NewLock(),
		}),
		expectedManifoldsWithDependencies,
	)
}

// TestControllerAgentConfigRequiresUnlocker verifies the safemode wiring
// must supply a non-nil ControllerUnlocker: without it the
// controller-agent-config manifold fails validation with a nil
// ReadyUnlocker error instead of starting.
func (s *ManifoldsSuite) TestControllerAgentConfigRequiresUnlocker(c *tc.C) {
	manifolds := safemode.Manifolds(safemode.ManifoldsConfig{
		Agent:                  &mockAgent{},
		ControllerID:           "0",
		ConfigChangeSocketPath: "configchange.socket",
	})

	getter := dependencytesting.StubGetter(map[string]any{
		"is-controller-flag": engine.NewStaticFlagWorker(true),
	})

	worker, err := manifolds["controller-agent-config"].Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, ".*nil ReadyUnlocker.*")
}

var expectedManifoldsWithDependencies = map[string][]string{

	"agent": {},

	"controller-agent-config": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"db-accessor": {
		"agent",
		"controller-agent-config",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"is-controller-flag": {"agent", "state-config-watcher"},

	"query-logger": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"state-config-watcher": {"agent"},

	"termination-signal-handler": {},
}

type mockAgent struct {
	agent.Agent
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
