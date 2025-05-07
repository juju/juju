// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	coretesting "github.com/juju/juju/internal/testing"
)

type WrapAgentSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&WrapAgentSuite{})

func (s *WrapAgentSuite) TestRequiresControllerUUID(c *tc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, "lol-nope-no-hope", coretesting.ModelTag.Id())
	c.Check(err, tc.ErrorMatches, `controller uuid "lol-nope-no-hope" not valid`)
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(agent, tc.IsNil)
}

func (s *WrapAgentSuite) TestRequiresModelUUID(c *tc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, coretesting.ControllerTag.Id(), "lol-nope-no-hope")
	c.Check(err, tc.ErrorMatches, `model uuid "lol-nope-no-hope" not valid`)
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(agent, tc.IsNil)
}

func (s *WrapAgentSuite) TestWraps(c *tc.C) {
	agent, err := model.WrapAgent(&mockAgent{}, coretesting.ControllerTag.Id(), coretesting.ModelTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	config := agent.CurrentConfig()

	c.Check(config.Model(), tc.Equals, coretesting.ModelTag)
	c.Check(config.Controller(), tc.Equals, coretesting.ControllerTag)
	c.Check(config.OldPassword(), tc.Equals, "")

	apiInfo, ok := config.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(apiInfo, tc.DeepEquals, &api.Info{
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
