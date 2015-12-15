// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type undertakerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *undertakerSuite) TestPermDenied(c *gc.C) {
	nonManagerMachine, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	for _, conn := range []api.Connection{
		nonManagerMachine,
		s.APIState,
	} {
		undertakerClient := undertaker.NewClient(conn)
		c.Assert(undertakerClient, gc.NotNil)

		_, err := undertakerClient.EnvironInfo()
		c.Assert(err, gc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestStateEnvionInfo(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)

	result, err := undertakerClient.EnvironInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	info := result.Result
	c.Assert(info.UUID, gc.Equals, coretesting.EnvironmentTag.Id())
	c.Assert(info.Name, gc.Equals, "dummyenv")
	c.Assert(info.GlobalName, gc.Equals, "user-dummy-admin@local/dummyenv")
	c.Assert(info.IsSystem, jc.IsTrue)
	c.Assert(info.Life, gc.Equals, params.Alive)
	c.Assert(info.TimeOfDeath, gc.IsNil)
}

func (s *undertakerSuite) TestStateProcessDyingEnviron(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)

	err := undertakerClient.ProcessDyingEnviron()
	c.Assert(err, gc.ErrorMatches, "environment is not dying")

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	err = undertakerClient.ProcessDyingEnviron()
	c.Assert(err, gc.ErrorMatches, `environment not empty, found 1 machine\(s\)`)
}

func (s *undertakerSuite) TestStateRemoveEnvironFails(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)
	c.Assert(undertakerClient.RemoveEnviron(), gc.ErrorMatches, "an error occurred, unable to remove environment")
}

func (s *undertakerSuite) TestHostedEnvironInfo(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	result, err := undertakerClient.EnvironInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	envInfo := result.Result
	c.Assert(envInfo.UUID, gc.Equals, otherSt.EnvironUUID())
	c.Assert(envInfo.Name, gc.Equals, "hosted_env")
	c.Assert(envInfo.GlobalName, gc.Equals, "user-dummy-admin@local/hosted_env")
	c.Assert(envInfo.IsSystem, jc.IsFalse)
	c.Assert(envInfo.Life, gc.Equals, params.Alive)
	c.Assert(envInfo.TimeOfDeath, gc.IsNil)
}

func (s *undertakerSuite) TestHostedProcessDyingEnviron(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	err := undertakerClient.ProcessDyingEnviron()
	c.Assert(err, gc.ErrorMatches, "environment is not dying")

	env, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	c.Assert(undertakerClient.ProcessDyingEnviron(), jc.ErrorIsNil)

	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)

	result, err := undertakerClient.EnvironInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	info := result.Result
	c.Assert(info.TimeOfDeath.IsZero(), jc.IsFalse)
}

func (s *undertakerSuite) TestWatchEnvironResources(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	w, err := undertakerClient.WatchEnvironResources()
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)

	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *undertakerSuite) TestHostedRemoveEnviron(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	// Aborts on alive environ.
	err := undertakerClient.RemoveEnviron()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove environment")

	env, err := otherSt.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)

	// Aborts on dying environ.
	err = undertakerClient.RemoveEnviron()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove environment")

	c.Assert(undertakerClient.ProcessDyingEnviron(), jc.ErrorIsNil)

	c.Assert(undertakerClient.RemoveEnviron(), jc.ErrorIsNil)
	c.Assert(otherSt.EnsureEnvironmentRemoved(), jc.ErrorIsNil)
}

func (s *undertakerSuite) hostedAPI(c *gc.C) (*undertaker.Client, *state.State) {
	otherState := s.Factory.MakeEnvironment(c, &factory.EnvParams{Name: "hosted_env"})

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:     []state.MachineJob{state.JobManageEnviron},
		Password: password,
		Nonce:    "fake_nonce",
	})

	// Connect to hosted environ from state server.
	info := s.APIInfo(c)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.EnvironTag = otherState.EnvironTag()

	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)

	undertakerClient := undertaker.NewClient(otherAPIState)
	c.Assert(undertakerClient, gc.NotNil)

	return undertakerClient, otherState
}
