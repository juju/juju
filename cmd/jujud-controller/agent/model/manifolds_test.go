// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestIAASNames(c *tc.C) {
	actual := set.NewStrings()
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: internallogger.DefaultContext(),
	})
	for name := range manifolds {
		actual.Add(name)
	}
	// NOTE: if this test failed, the cmd/jujud-controller/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.SameContents, []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"async-charm-downloader",
		"charm-revisioner",
		"clock",
		"compute-provisioner",
		"domain-services",
		"firewaller",
		"http-client",
		"instance-mutater",
		"instance-poller",
		"is-responsible-flag",
		"lease-manager",
		"logging-config-updater",
		"machine-undertaker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"not-alive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"remote-relations",
		"removal",
		"secrets-pruner",
		"state-cleaner",
		"storage-provisioner",
		"undertaker",
		"unit-assigner",
		"user-secrets-drain-worker",
		"valid-credential-flag",
	})
}

func (s *ManifoldsSuite) TestCAASNames(c *tc.C) {
	actual := set.NewStrings()
	manifolds := model.CAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: internallogger.DefaultContext(),
	})
	for name := range manifolds {
		actual.Add(name)
	}
	// NOTE: if this test failed, the cmd/jujud-controller/agent tests will
	// also fail. Search for 'ModelWorkers' to find affected vars.
	c.Check(actual.SortedValues(), jc.SameContents, []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"async-charm-downloader",
		"caas-application-provisioner",
		"caas-firewaller",
		"caas-model-config-manager",
		"caas-model-operator",
		"caas-storage-provisioner",
		"charm-revisioner",
		"clock",
		"domain-services",
		"http-client",
		"is-responsible-flag",
		"lease-manager",
		"logging-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"not-alive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"remote-relations",
		"removal",
		"secrets-pruner",
		"state-cleaner",
		"undertaker",
		"user-secrets-drain-worker",
		"valid-credential-flag",
	})
}

func (s *ManifoldsSuite) TestFlagDependencies(c *tc.C) {
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
		"domain-services",
		"lease-manager",
		"http-client",
		"valid-credential-flag",
	)
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: internallogger.DefaultContext(),
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

func (s *ManifoldsSuite) TestStateCleanerIgnoresLifeFlags(c *tc.C) {
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		LoggingContext: internallogger.DefaultContext(),
	})
	manifold, found := manifolds["state-cleaner"]
	c.Assert(found, jc.IsTrue)

	inputs := set.NewStrings(manifold.Inputs...)
	c.Check(inputs.Contains("not-alive-flag"), jc.IsFalse)
	c.Check(inputs.Contains("not-dead-flag"), jc.IsFalse)
}

func (s *ManifoldsSuite) TestClockWrapper(c *tc.C) {
	expectClock := &fakeClock{}
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent:          &mockAgent{},
		Clock:          expectClock,
		LoggingContext: internallogger.DefaultContext(),
	})
	manifold, ok := manifolds["clock"]
	c.Assert(ok, jc.IsTrue)
	worker, err := manifold.Start(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, worker)

	var aClock clock.Clock
	err = manifold.Output(worker, &aClock)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(aClock, tc.Equals, expectClock)
}

type fakeClock struct{ clock.Clock }

func (s *ManifoldsSuite) TestIAASManifold(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		model.IAASManifolds(model.ManifoldsConfig{
			Agent:          &mockAgent{},
			LoggingContext: internallogger.DefaultContext(),
		}),
		expectedIAASModelManifoldsWithDependencies,
	)
}

func (s *ManifoldsSuite) TestCAASManifold(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		model.CAASManifolds(model.ManifoldsConfig{
			Agent:          &mockAgent{},
			LoggingContext: internallogger.DefaultContext(),
		}),
		expectedCAASModelManifoldsWithDependencies,
	)
}

var expectedCAASModelManifoldsWithDependencies = map[string][]string{

	"secrets-pruner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"user-secrets-drain-worker": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"agent": {},

	"api-caller": {"agent"},

	"api-config-watcher": {"agent"},

	"provider-tracker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"lease-manager",
		"provider-service-factories",
		"valid-credential-flag",
	},

	"caas-model-config-manager": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"lease-manager",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"caas-firewaller": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"caas-model-operator": {
		"agent",
		"api-caller",
		"provider-service-factories",
		"provider-tracker",
		"is-responsible-flag",
		"lease-manager",
		"valid-credential-flag",
	},

	"caas-application-provisioner": {
		"agent",
		"api-caller",
		"clock",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"caas-storage-provisioner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"async-charm-downloader": {
		"agent",
		"domain-services",
		"http-client",
		"is-responsible-flag",
		"lease-manager",
	},

	"charm-revisioner": {
		"agent",
		"domain-services",
		"http-client",
		"is-responsible-flag",
		"lease-manager",
	},

	"clock": {},

	"is-responsible-flag": {"agent", "lease-manager"},

	"lease-manager": {},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"migration-fortress": {
		"agent",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-dead-flag",
	},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-dead-flag",
	},

	"migration-master": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"not-dead-flag",
	},

	"not-alive-flag": {
		"domain-services",
	},

	"not-dead-flag": {
		"domain-services",
	},

	"provider-service-factories": {},

	"remote-relations": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"removal": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"domain-services": {},

	"http-client": {},

	"state-cleaner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"undertaker": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-alive-flag",
	},

	"valid-credential-flag": {"agent", "api-caller"},
}

var expectedIAASModelManifoldsWithDependencies = map[string][]string{

	"secrets-pruner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"user-secrets-drain-worker": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"agent": {},

	"api-caller": {"agent"},

	"api-config-watcher": {"agent"},

	"async-charm-downloader": {
		"agent",
		"lease-manager",
		"domain-services",
		"http-client",
		"is-responsible-flag",
	},

	"charm-revisioner": {
		"agent",
		"lease-manager",
		"domain-services",
		"http-client",
		"is-responsible-flag",
	},

	"clock": {},

	"compute-provisioner": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"domain-services",
		"valid-credential-flag",
	},

	"provider-tracker": {
		"agent",
		"api-caller",
		"is-responsible-flag",
		"lease-manager",
		"provider-service-factories",
		"valid-credential-flag",
	},

	"firewaller": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"instance-mutater": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"instance-poller": {
		"agent",
		"api-caller",
		"clock",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"is-responsible-flag": {"agent", "lease-manager"},

	"lease-manager": {},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"machine-undertaker": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"migration-fortress": {
		"agent",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-dead-flag",
	},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-dead-flag",
	},

	"migration-master": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"not-dead-flag",
	},

	"not-alive-flag": {
		"domain-services",
	},

	"not-dead-flag": {
		"domain-services",
	},

	"provider-service-factories": {},

	"remote-relations": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"removal": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"domain-services": {},

	"http-client": {},

	"state-cleaner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"storage-provisioner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-tracker",
		"valid-credential-flag",
	},

	"undertaker": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"not-alive-flag",
	},

	"unit-assigner": {
		"agent",
		"api-caller",
		"domain-services",
		"is-responsible-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"not-dead-flag",
	},

	"valid-credential-flag": {"agent", "api-caller"},
}
