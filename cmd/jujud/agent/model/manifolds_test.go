// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/set"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/model"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestNames(c *gc.C) {
	actual := set.NewStrings()
	manifolds := model.Manifolds(model.ManifoldsConfig{})
	for name := range manifolds {
		actual.Add(name)
	}
	c.Check(actual.Values(), jc.SameContents, []string{
		"agent", "clock", "api-caller", "run-flag",
		"storage-provisioner", "compute-provisioner",
		"firewaller", "unit-assigner", "service-scaler",
		"instance-poller", "charm-revision-updater",
		"metric-worker", "state-cleaner", "address-cleaner",
		"status-history-pruner",
	})
}

func (s *ManifoldsSuite) TestRunFlagDependencies(c *gc.C) {
	exclusions := set.NewStrings("agent", "api-caller", "clock", "run-flag")
	manifolds := model.Manifolds(model.ManifoldsConfig{})
	for name, manifold := range manifolds {
		c.Logf("checking %s", name)
		if exclusions.Contains(name) {
			continue
		}
		inputs := set.NewStrings(manifold.Inputs...)
		c.Check(inputs.Contains("run-flag"), jc.IsTrue)
	}
}

func (s *ManifoldsSuite) TestModelAgentWrapper(c *gc.C) {
	manifolds := model.Manifolds(model.ManifoldsConfig{
		Agent:     &mockAgent{},
		ModelUUID: coretesting.ModelTag.Id(),
	})
	manifold, ok := manifolds["agent"]
	c.Assert(ok, jc.IsTrue)
	worker, err := manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, worker)

	var agent agent.Agent
	err = manifold.Output(worker, &agent)
	c.Assert(err, jc.ErrorIsNil)
	config := agent.CurrentConfig()

	c.Check(config.Model(), gc.Equals, coretesting.ModelTag)
	c.Check(config.OldPassword(), gc.Equals, "")
	apiInfo, ok := config.APIInfo()
	c.Assert(ok, jc.IsTrue)
	c.Check(apiInfo, gc.DeepEquals, &api.Info{
		Addrs:    []string{"here", "there"},
		CACert:   "trust-me",
		ModelTag: coretesting.ModelTag,
		Tag:      names.NewMachineTag("123"),
		Password: "12345",
		Nonce:    "11111",
	})
}

func (s *ManifoldsSuite) TestClockWrapper(c *gc.C) {
	expectClock := &fakeClock{}
	manifolds := model.Manifolds(model.ManifoldsConfig{
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

type mockAgent struct{ agent.Agent }

func (mock *mockAgent) CurrentConfig() agent.Config {
	return &mockConfig{}
}

type mockConfig struct{ agent.Config }

func (mock *mockConfig) Model() names.ModelTag {
	return names.NewModelTag("bad-wrong-no")
}

func (mock *mockConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		Addrs:    []string{"here", "there"},
		CACert:   "trust-me",
		ModelTag: names.NewModelTag("bad-wrong-no"),
		Tag:      names.NewMachineTag("123"),
		Password: "12345",
		Nonce:    "11111",
	}, true
}

func (mock *mockConfig) OldPassword() string {
	return "do-not-use"
}

type fakeClock struct{ clock.Clock }
