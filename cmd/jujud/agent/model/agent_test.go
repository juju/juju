// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/model"
	coretesting "github.com/juju/juju/testing"
)

type WrapAgentSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WrapAgentSuite{})

func (s *WrapAgentSuite) TestRequiresControllerUUID(c *gc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, "lol-nope-no-hope", coretesting.ModelTag.Id())
	c.Check(err, gc.ErrorMatches, `controller uuid "lol-nope-no-hope" not valid`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(agent, gc.IsNil)
}

func (s *WrapAgentSuite) TestRequiresModelUUID(c *gc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, coretesting.ControllerTag.Id(), "lol-nope-no-hope")
	c.Check(err, gc.ErrorMatches, `model uuid "lol-nope-no-hope" not valid`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(agent, gc.IsNil)
}

func (s *WrapAgentSuite) TestWraps(c *gc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, coretesting.ControllerTag.Id(), coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	config := agent.CurrentConfig()

	c.Check(config.Model(), gc.Equals, coretesting.ModelTag)
	c.Check(config.Controller(), gc.Equals, coretesting.ControllerTag)
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

type mockAgent struct{ agent.Agent }

func (mock *mockAgent) CurrentConfig() agent.Config {
	return &mockConfig{}
}

type mockConfig struct{ agent.Config }

func (mock *mockConfig) Tag() names.Tag {
	return names.NewMachineTag("123")
}

func (mock *mockConfig) Model() names.ModelTag {
	return names.NewModelTag("mock-model-uuid")
}

func (mock *mockConfig) Controller() names.ControllerTag {
	return names.NewControllerTag("mock-controller-uuid")
}

func (mock *mockConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		Addrs:    []string{"here", "there"},
		CACert:   "trust-me",
		ModelTag: names.NewModelTag("mock-model-uuid"),
		Tag:      names.NewMachineTag("123"),
		Password: "12345",
		Nonce:    "11111",
	}, true
}

func (mock *mockConfig) OldPassword() string {
	return "do-not-use"
}
