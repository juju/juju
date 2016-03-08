// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/undertaker"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type undertakerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) setupStateAndAPI(c *gc.C, isSystem bool, envName string) (*mockState, *undertaker.UndertakerAPI) {
	machineNo := "1"
	if isSystem {
		machineNo = "0"
	}

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag(machineNo),
		EnvironManager: true,
	}

	st := newMockState(names.NewUserTag("admin"), envName, isSystem)
	api, err := undertaker.NewUndertaker(st, nil, authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return st, api
}

func (s *undertakerSuite) TestNoPerms(c *gc.C) {
	for _, authorizer := range []apiservertesting.FakeAuthorizer{
		apiservertesting.FakeAuthorizer{
			Tag: names.NewMachineTag("0"),
		},
		apiservertesting.FakeAuthorizer{
			Tag:            names.NewUserTag("bob"),
			EnvironManager: true,
		},
	} {
		st := newMockState(names.NewUserTag("admin"), "dummymodel", true)
		_, err := undertaker.NewUndertaker(
			st,
			nil,
			authorizer,
		)
		c.Assert(err, gc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestEnvironInfo(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	st, api := s.setupStateAndAPI(c, true, "dummymodel")
	for _, test := range []struct {
		st       *mockState
		api      *undertaker.UndertakerAPI
		isSystem bool
		envName  string
	}{
		{otherSt, hostedAPI, false, "hostedenv"},
		{st, api, true, "dummymodel"},
	} {
		env, err := test.st.Model()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(env.Destroy(), jc.ErrorIsNil)

		result, err := test.api.ModelInfo()
		c.Assert(err, jc.ErrorIsNil)

		info := result.Result
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)

		c.Assert(info.UUID, gc.Equals, env.UUID())
		c.Assert(info.GlobalName, gc.Equals, "user-admin/"+test.envName)
		c.Assert(info.Name, gc.Equals, test.envName)
		c.Assert(info.IsSystem, gc.Equals, test.isSystem)
		c.Assert(info.Life, gc.Equals, params.Dying)
		c.Assert(info.TimeOfDeath, gc.IsNil)
	}
}

func (s *undertakerSuite) TestProcessDyingEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")
	c.Assert(env.Life(), gc.Equals, state.Alive)

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Life(), gc.Equals, state.Dying)

	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	_, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove model")
}

func (s *undertakerSuite) TestRemoveDyingEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Set env to dying
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove model")
}

func (s *undertakerSuite) TestDeadRemoveEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Set env to dead
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
}

func (s *undertakerSuite) TestModelConfig(c *gc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")

	cfg, err := hostedAPI.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.NotNil)
}
