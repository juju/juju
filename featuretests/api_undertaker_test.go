// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/watcher/watchertest"
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

		_, err := undertakerClient.ModelInfo()
		c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
			Message: "permission denied",
			Code:    "unauthorized access",
		})
	}
}

func (s *undertakerSuite) TestStateEnvironInfo(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	info := result.Result
	c.Assert(info.UUID, gc.Equals, coretesting.ModelTag.Id())
	c.Assert(info.Name, gc.Equals, "dummymodel")
	c.Assert(info.GlobalName, gc.Equals, "user-admin@local/dummymodel")
	c.Assert(info.IsSystem, jc.IsTrue)
	c.Assert(info.Life, gc.Equals, params.Alive)
	c.Assert(info.TimeOfDeath, gc.IsNil)
}

func (s *undertakerSuite) TestStateProcessDyingEnviron(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)

	err := undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	err = undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, `model not empty, found 1 machine\(s\)`)
}

func (s *undertakerSuite) TestStateRemoveEnvironFails(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient := undertaker.NewClient(st)
	c.Assert(undertakerClient, gc.NotNil)
	c.Assert(undertakerClient.RemoveModel(), gc.ErrorMatches, "an error occurred, unable to remove model")
}

func (s *undertakerSuite) TestHostedEnvironInfo(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	envInfo := result.Result
	c.Assert(envInfo.UUID, gc.Equals, otherSt.ModelUUID())
	c.Assert(envInfo.Name, gc.Equals, "hosted_env")
	c.Assert(envInfo.GlobalName, gc.Equals, "user-admin@local/hosted_env")
	c.Assert(envInfo.IsSystem, jc.IsFalse)
	c.Assert(envInfo.Life, gc.Equals, params.Alive)
	c.Assert(envInfo.TimeOfDeath, gc.IsNil)
}

func (s *undertakerSuite) TestHostedProcessDyingEnviron(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	err := undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	env, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	c.Assert(undertakerClient.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	info := result.Result
	c.Assert(info.TimeOfDeath.IsZero(), jc.IsFalse)
}

func (s *undertakerSuite) TestWatchModelResources(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	w, err := undertakerClient.WatchModelResources()
	c.Assert(err, jc.ErrorIsNil)
	defer w.Kill()
	wc := watchertest.NewNotifyWatcherC(c, w, nil)
	wc.AssertOneChange()
	wc.AssertStops()
}

func (s *undertakerSuite) TestHostedRemoveEnviron(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	// Aborts on alive environ.
	err := undertakerClient.RemoveModel()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove model")

	env, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)

	// Aborts on dying environ.
	err = undertakerClient.RemoveModel()
	c.Assert(err, gc.ErrorMatches, "an error occurred, unable to remove model")

	c.Assert(undertakerClient.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(undertakerClient.RemoveModel(), jc.ErrorIsNil)
	c.Assert(otherSt.EnsureModelRemoved(), jc.ErrorIsNil)
}

func (s *undertakerSuite) TestHostedModelConfig(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	cfg, err := undertakerClient.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	uuid, ok := cfg.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, otherSt.ModelUUID())
}

func (s *undertakerSuite) hostedAPI(c *gc.C) (*undertaker.Client, *state.State) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "hosted_env"})

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:     []state.MachineJob{state.JobManageModel},
		Password: password,
		Nonce:    "fake_nonce",
	})

	// Connect to hosted environ from controller.
	info := s.APIInfo(c)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = otherState.ModelTag()

	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)

	undertakerClient := undertaker.NewClient(otherAPIState)
	c.Assert(undertakerClient, gc.NotNil)

	return undertakerClient, otherState
}
