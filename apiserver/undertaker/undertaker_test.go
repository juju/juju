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

	st := newMockState(names.NewUserTag("dummy-admin"), envName, isSystem)
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
		st := newMockState(names.NewUserTag("dummy-admin"), "dummyenv", true)
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
	st, api := s.setupStateAndAPI(c, true, "dummyenv")
	for _, test := range []struct {
		st       *mockState
		api      *undertaker.UndertakerAPI
		isSystem bool
		envName  string
	}{
		{otherSt, hostedAPI, false, "hostedenv"},
		{st, api, true, "dummyenv"},
	} {
		env, err := test.st.Environment()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(env.Destroy(), jc.ErrorIsNil)

		result, err := test.api.EnvironInfo()
		c.Assert(err, jc.ErrorIsNil)

		info := result.Result
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)

		c.Assert(info.UUID, gc.Equals, env.UUID())
		c.Assert(info.GlobalName, gc.Equals, "user-dummy-admin/"+test.envName)
		c.Assert(info.Name, gc.Equals, test.envName)
		c.Assert(info.IsSystem, gc.Equals, test.isSystem)
		c.Assert(info.Life, gc.Equals, params.Dying)
		c.Assert(info.TimeOfDeath, gc.IsNil)
	}
}

func (s *undertakerSuite) TestProcessDyingEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.ProcessDyingEnviron()
	c.Assert(err, gc.ErrorMatches, "environment is not dying")
	c.Assert(env.Life(), gc.Equals, state.Alive)

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Life(), gc.Equals, state.Dying)

	err = hostedAPI.ProcessDyingEnviron()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	_, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveEnviron()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove environment")
}

func (s *undertakerSuite) TestRemoveDyingEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Set env to dying
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveEnviron()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove environment")
}

func (s *undertakerSuite) TestDeadRemoveEnviron(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedenv")
	env, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Set env to dead
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = hostedAPI.ProcessDyingEnviron()
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveEnviron()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
}
