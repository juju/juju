// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestIAASNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: loggo.DefaultContext(),
	})
	for name := range manifolds {
		actual.Add(name)
	}
	// NOTE: if this test failed, the cmd/jujud/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.DeepEquals, []string{
		"action-pruner",
		"agent",
		"api-caller",
		"api-config-watcher",
		"application-scaler",
		"charm-revision-updater",
		"clock",
		"compute-provisioner",
		"environ-tracker",
		"firewaller",
		"instance-mutater",
		"instance-poller",
		"is-responsible-flag",
		"log-forwarder",
		"logging-config-updater",
		"machine-undertaker",
		"metric-worker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"model-upgrader",
		"not-alive-flag",
		"not-dead-flag",
		"remote-relations",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"undertaker",
		"unit-assigner",
		"valid-credential-flag",
	})
}

func (s *ManifoldsSuite) TestCAASNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.CAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: loggo.DefaultContext(),
	})
	for name := range manifolds {
		actual.Add(name)
	}
	// NOTE: if this test failed, the cmd/jujud/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.DeepEquals, []string{
		"action-pruner",
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-admission",
		"caas-broker-tracker",
		"caas-firewaller",
		"caas-operator-provisioner",
		"caas-rbac-mapper",
		"caas-storage-provisioner",
		"caas-unit-provisioner",
		"charm-revision-updater",
		"clock",
		"is-responsible-flag",
		"log-forwarder",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"model-upgrader",
		"not-alive-flag",
		"not-dead-flag",
		"remote-relations",
		"state-cleaner",
		"status-history-pruner",
		"undertaker",
		"valid-credential-flag",
	})
}

func (s *ManifoldsSuite) TestFlagDependencies(c *gc.C) {
	exclusions := set.NewStrings(
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-responsible-flag",
		"not-alive-flag",
		"not-dead-flag",
		// model upgrade manifolds are run on all
		// controller agents, "responsible" or not.
		"model-upgrade-gate",
		"model-upgraded-flag",
		"model-upgrader",
		"valid-credential-flag",
	)
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: loggo.DefaultContext(),
	})
	for name, manifold := range manifolds {
		c.Logf("checking %s", name)
		if exclusions.Contains(name) {
			continue
		}
		inputs := set.NewStrings(manifold.Inputs...)
		if !inputs.Contains("is-responsible-flag") {
			c.Check(inputs.Contains("migration-fortress"), jc.IsTrue)
			c.Check(inputs.Contains("migration-inactive-flag"), jc.IsTrue)
		}
	}
}

func (s *ManifoldsSuite) TestStateCleanerIgnoresLifeFlags(c *gc.C) {
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: loggo.DefaultContext(),
	})
	manifold, found := manifolds["state-cleaner"]
	c.Assert(found, jc.IsTrue)

	inputs := set.NewStrings(manifold.Inputs...)
	c.Check(inputs.Contains("not-alive-flag"), jc.IsFalse)
	c.Check(inputs.Contains("not-dead-flag"), jc.IsFalse)
}

func (s *ManifoldsSuite) TestClockWrapper(c *gc.C) {
	expectClock := &fakeClock{}
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		Clock:          expectClock,
		LoggingContext: loggo.DefaultContext(),
	})
	manifold, ok := manifolds["clock"]
	c.Assert(ok, jc.IsTrue)
	worker, err := manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, worker)

	var aClock clock.Clock
	err = manifold.Output(worker, &aClock)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(aClock, gc.Equals, expectClock)
}

type fakeClock struct{ clock.Clock }

func (s *ManifoldsSuite) TestIAASManifold(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		model.IAASManifolds(model.ManifoldsConfig{
			Agent:          &mockAgent{},
			LoggingContext: loggo.DefaultContext(),
		}),
		expectedIAASModelManifoldsWithDependencies,
	)
}

func (s *ManifoldsSuite) TestCAASManifold(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		model.CAASManifolds(model.ManifoldsConfig{
			Agent:          &mockAgent{},
			LoggingContext: loggo.DefaultContext(),
		}),
		expectedCAASModelManifoldsWithDependencies,
	)
}

var expectedCAASModelManifoldsWithDependencies = map[string][]string{
	"action-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"agent": {},

	"api-caller": {"agent"},

	"api-config-watcher": {"agent"},

	"caas-admission": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"caas-rbac-mapper",
		"is-responsible-flag",
	},

	"caas-broker-tracker": {"agent", "api-caller", "is-responsible-flag"},

	"caas-firewaller": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"caas-operator-provisioner": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"caas-rbac-mapper": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"is-responsible-flag",
	},

	"caas-storage-provisioner": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag"},

	"caas-unit-provisioner": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"charm-revision-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"clock": {},

	"is-responsible-flag": {"agent", "api-caller"},

	"log-forwarder": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag"},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
	},

	"migration-fortress": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-master": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"model-upgrade-gate": {},

	"model-upgraded-flag": {"model-upgrade-gate"},

	"model-upgrader": {"agent", "api-caller", "model-upgrade-gate"},

	"not-alive-flag": {"agent", "api-caller"},

	"not-dead-flag": {"agent", "api-caller"},

	"remote-relations": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"state-cleaner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"status-history-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"undertaker": {
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-alive-flag",
	},

	"valid-credential-flag": {"agent", "api-caller"},
}

var expectedIAASModelManifoldsWithDependencies = map[string][]string{

	"action-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
	},

	"agent": {},

	"api-caller": {"agent"},

	"api-config-watcher": {"agent"},

	"application-scaler": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"charm-revision-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"clock": {},

	"compute-provisioner": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"environ-tracker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"valid-credential-flag",
	},

	"firewaller": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"instance-mutater": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"instance-poller": {
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"is-responsible-flag": {"agent", "api-caller"},

	"log-forwarder": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag",
	},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
	},

	"machine-undertaker": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"metric-worker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-fortress": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-master": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"model-upgrade-gate": {},

	"model-upgraded-flag": {"model-upgrade-gate"},

	"model-upgrader": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"model-upgrade-gate",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"not-alive-flag": {"agent", "api-caller"},

	"not-dead-flag": {"agent", "api-caller"},

	"remote-relations": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"state-cleaner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"status-history-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
	},

	"storage-provisioner": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag",
	},

	"undertaker": {
		"agent",
		"api-caller",
		"environ-tracker",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-alive-flag",
		"valid-credential-flag",
	},

	"unit-assigner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"valid-credential-flag": {"agent", "api-caller"},
}
