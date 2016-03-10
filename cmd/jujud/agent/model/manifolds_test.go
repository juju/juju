// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.Manifolds(model.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	for name := range manifolds {
		actual.Add(name)
	}
	c.Check(actual.Values(), jc.SameContents, []string{
		"agent", "clock", "api-caller",
		"run-flag", "not-dead-flag", "not-alive-flag",
		"environ-tracker", "undertaker", "space-importer",
		"storage-provisioner", "compute-provisioner",
		"firewaller", "unit-assigner", "service-scaler",
		"instance-poller", "charm-revision-updater",
		"metric-worker", "state-cleaner", "address-cleaner",
		"status-history-pruner",
	})
}

func (s *ManifoldsSuite) TestRunFlagDependencies(c *gc.C) {
	exclusions := set.NewStrings(
		"agent", "api-caller", "clock",
		"run-flag", "not-dead-flag", "not-alive-flag",
	)
	manifolds := model.Manifolds(model.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	for name, manifold := range manifolds {
		c.Logf("checking %s", name)
		if exclusions.Contains(name) {
			continue
		}
		inputs := set.NewStrings(manifold.Inputs...)
		c.Check(inputs.Contains("run-flag"), jc.IsTrue)
	}
}

func (s *ManifoldsSuite) TestClockWrapper(c *gc.C) {
	expectClock := &fakeClock{}
	manifolds := model.Manifolds(model.ManifoldsConfig{
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
