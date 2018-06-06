// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestIAASNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent: &mockAgent{},
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
		"credential-validator-flag",
		"environ-tracker",
		"firewaller",
		"instance-poller",
		"is-responsible-flag",
		"log-forwarder",
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
	})
}

func (s *ManifoldsSuite) TestCAASNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.CAASManifolds(model.ManifoldsConfig{
		Agent: &mockAgent{},
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
		"caas-broker-tracker",
		"caas-firewaller",
		"caas-operator-provisioner",
		"caas-unit-provisioner",
		"charm-revision-updater",
		"clock",
		"credential-validator-flag",
		"is-responsible-flag",
		"log-forwarder",
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
	})
}

func (s *ManifoldsSuite) TestFlagDependencies(c *gc.C) {
	exclusions := set.NewStrings(
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"credential-validator-flag",
		"is-responsible-flag",
		"not-alive-flag",
		"not-dead-flag",
		// model upgrade manifolds are run on all
		// controller agents, "responsible" or not.
		"model-upgrade-gate",
		"model-upgraded-flag",
		"model-upgrader",
	)
	manifolds := model.IAASManifolds(model.ManifoldsConfig{
		Agent: &mockAgent{},
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
		Agent: &mockAgent{},
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
		Agent: &mockAgent{},
		Clock: expectClock,
	})
	manifold, ok := manifolds["clock"]
	c.Assert(ok, jc.IsTrue)
	worker, err := manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, worker)

	var clock clock.Clock
	err = manifold.Output(worker, &clock)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(clock, gc.Equals, expectClock)
}

type fakeClock struct{ clock.Clock }

func (s *ManifoldsSuite) assertManifoldsDependencies(c *gc.C, manifolds dependency.Manifolds, expected map[string][]string) {
	dependencies := make(map[string][]string, len(manifolds))
	manifoldNames := set.NewStrings()

	for name, manifold := range manifolds {
		manifoldNames.Add(name)
		dependencies[name] = manifolds.ManifoldDependencies(name, manifold).SortedValues()
	}
	c.Assert(len(dependencies), gc.Equals, len(expected))

	for _, n := range manifoldNames.SortedValues() {
		c.Assert(dependencies[n], jc.SameContents, expected[n])
	}
}

func (s *ManifoldsSuite) TestIAASManifold(c *gc.C) {
	s.assertManifoldsDependencies(c,
		model.IAASManifolds(model.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		expectedIAASModelManifoldsWithDependencies,
	)
}

func (s *ManifoldsSuite) TestCAASManifold(c *gc.C) {
	s.assertManifoldsDependencies(c,
		model.CAASManifolds(model.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		expectedCAASModelManifoldsWithDependencies,
	)
}

var expectedCAASModelManifoldsWithDependencies = map[string][]string{
	"action-pruner": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"agent": []string{},

	"api-caller": []string{"agent"},

	"api-config-watcher": []string{"agent"},

	"caas-broker-tracker": []string{"agent", "api-caller", "clock", "is-responsible-flag"},

	"caas-firewaller": []string{
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

	"caas-operator-provisioner": []string{
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

	"caas-unit-provisioner": []string{
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

	"charm-revision-updater": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"clock": []string{},

	"credential-validator-flag": []string{"agent", "api-caller"},

	"is-responsible-flag": []string{"agent", "api-caller", "clock"},

	"log-forwarder": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"not-dead-flag"},

	"migration-fortress": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-inactive-flag": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-master": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"model-upgrade-gate": []string{},

	"model-upgraded-flag": []string{"model-upgrade-gate"},

	"model-upgrader": []string{"agent", "api-caller", "model-upgrade-gate"},

	"not-alive-flag": []string{"agent", "api-caller"},

	"not-dead-flag": []string{"agent", "api-caller"},

	"remote-relations": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"state-cleaner": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"status-history-pruner": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"undertaker": []string{
		"agent",
		"api-caller",
		"caas-broker-tracker",
		"clock",
		"credential-validator-flag",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-alive-flag"},
}

var expectedIAASModelManifoldsWithDependencies = map[string][]string{

	"action-pruner": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"agent": []string{},

	"api-caller": []string{"agent"},

	"api-config-watcher": []string{"agent"},

	"application-scaler": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"charm-revision-updater": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"clock": []string{},

	"compute-provisioner": []string{
		"agent",
		"api-caller",
		"clock",
		"credential-validator-flag",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"credential-validator-flag": []string{"agent", "api-caller"},

	"environ-tracker": []string{"agent", "api-caller", "clock", "is-responsible-flag"},

	"firewaller": []string{
		"agent",
		"api-caller",
		"clock",
		"credential-validator-flag",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"instance-poller": []string{
		"agent",
		"api-caller",
		"clock",
		"credential-validator-flag",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"is-responsible-flag": []string{"agent", "api-caller", "clock"},

	"log-forwarder": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"not-dead-flag"},

	"machine-undertaker": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"metric-worker": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-fortress": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-inactive-flag": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"migration-master": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"model-upgrade-gate": []string{},

	"model-upgraded-flag": []string{"model-upgrade-gate"},

	"model-upgrader": []string{
		"agent",
		"api-caller",
		"clock",
		"credential-validator-flag",
		"environ-tracker",
		"is-responsible-flag",
		"model-upgrade-gate"},

	"not-alive-flag": []string{"agent", "api-caller"},

	"not-dead-flag": []string{"agent", "api-caller"},

	"remote-relations": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"state-cleaner": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"status-history-pruner": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"storage-provisioner": []string{
		"agent",
		"api-caller",
		"clock",
		"environ-tracker",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},

	"undertaker": []string{
		"agent",
		"api-caller",
		"clock",
		"credential-validator-flag",
		"environ-tracker",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-alive-flag"},

	"unit-assigner": []string{
		"agent",
		"api-caller",
		"clock",
		"is-responsible-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-dead-flag"},
}
