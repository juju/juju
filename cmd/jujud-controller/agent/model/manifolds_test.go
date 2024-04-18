// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
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
	// NOTE: if this test failed, the cmd/jujud-controller/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.DeepEquals, []string{
		"action-pruner",
		"agent",
		"api-caller",
		"api-config-watcher",
		"application-scaler",
		"charm-downloader",
		"charm-revision-updater",
		"clock",
		"compute-provisioner",
		"firewaller",
		"instance-mutater",
		"instance-poller",
		"is-responsible-flag",
		"logging-config-updater",
		"machine-undertaker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"not-alive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"provider-upgrader",
		"remote-relations",
		"secrets-pruner",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"undertaker",
		"unit-assigner",
		"user-secrets-drain-worker",
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
	// NOTE: if this test failed, the cmd/jujud-controller/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.DeepEquals, []string{
		"action-pruner",
		"agent",
		"api-caller",
		"api-config-watcher",
		"caas-application-provisioner",
		"caas-firewaller",
		"caas-model-config-manager",
		"caas-model-operator",
		"caas-storage-provisioner",
		"charm-downloader",
		"charm-revision-updater",
		"clock",
		"is-responsible-flag",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"not-alive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"provider-upgrader",
		"remote-relations",
		"secrets-pruner",
		"state-cleaner",
		"status-history-pruner",
		"undertaker",
		"user-secrets-drain-worker",
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
		"provider-service-factories",
		// model upgrade manifolds are run on all
		// controller agents, "responsible" or not.
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"provider-upgrader",
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
	worker, err := manifold.Start(context.Background(), nil)
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
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"secrets-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"user-secrets-drain-worker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"agent": {},

	"api-caller": {"agent"},

	"api-config-watcher": {"agent"},

	"provider-tracker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"provider-service-factories",
		"valid-credential-flag",
	},

	"caas-model-config-manager": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"caas-firewaller": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"caas-model-operator": {
		"agent",
		"api-caller",
		"provider-service-factories",
		"provider-tracker",
		"is-responsible-flag",
		"valid-credential-flag",
	},

	"caas-application-provisioner": {
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"caas-storage-provisioner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"charm-downloader": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"charm-revision-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"clock": {},

	"is-responsible-flag": {"agent", "api-caller"},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"migration-fortress": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"migration-master": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"provider-upgrade-gate": {},

	"provider-upgraded-flag": {"provider-upgrade-gate"},

	"provider-upgrader": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"valid-credential-flag",
	},

	"not-alive-flag": {"agent", "api-caller"},

	"not-dead-flag": {"agent", "api-caller"},

	"provider-service-factories": {},

	"remote-relations": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"state-cleaner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"status-history-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
	},

	"undertaker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
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
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag",
	},

	"secrets-pruner": {
		"agent",
		"api-caller",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"user-secrets-drain-worker": {
		"agent",
		"api-caller",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
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
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"charm-downloader": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag",
		"valid-credential-flag"},

	"charm-revision-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"clock": {},

	"compute-provisioner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"provider-tracker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"provider-service-factories",
		"valid-credential-flag",
	},

	"firewaller": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"instance-mutater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"instance-poller": {
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"is-responsible-flag": {"agent", "api-caller"},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag",
	},

	"machine-undertaker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"migration-fortress": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"migration-master": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"provider-upgrade-gate": {},

	"provider-upgraded-flag": {"provider-upgrade-gate"},

	"provider-upgrader": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"valid-credential-flag",
	},

	"not-alive-flag": {"agent", "api-caller"},

	"not-dead-flag": {"agent", "api-caller"},

	"provider-service-factories": {},

	"remote-relations": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"state-cleaner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"status-history-pruner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag",
	},

	"storage-provisioner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	},

	"undertaker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"not-alive-flag",
	},

	"unit-assigner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"not-dead-flag"},

	"valid-credential-flag": {"agent", "api-caller"},
}
