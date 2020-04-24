// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/undertaker"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// TODO(fwereade) 2016-03-17 lp:1558668
// this is not a feature test; much of it is redundant, and other
// bits should be tested elsewhere.
type undertakerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *undertakerSuite) TestPermDenied(c *gc.C) {
	nonManagerMachine, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	for _, conn := range []api.Connection{
		nonManagerMachine,
		s.APIState,
	} {
		undertakerClient, err := undertaker.NewClient(conn, apiwatcher.NewNotifyWatcher)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(undertakerClient, gc.NotNil)

		_, err = undertakerClient.ModelInfo()
		c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
			Message: "permission denied",
			Code:    "unauthorized access",
		})
	}
}

func (s *undertakerSuite) TestStateEnvironInfo(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient, gc.NotNil)

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Error, gc.IsNil)
	info := result.Result
	c.Assert(info.UUID, gc.Equals, coretesting.ModelTag.Id())
	c.Assert(info.Name, gc.Equals, "controller")
	c.Assert(info.GlobalName, gc.Equals, "user-admin/controller")
	c.Assert(info.IsSystem, jc.IsTrue)
	c.Assert(info.Life, gc.Equals, life.Alive)
}

func (s *undertakerSuite) TestStateProcessDyingEnviron(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient, gc.NotNil)

	err = undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	err = undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, `model not empty, found 1 machine \(model not empty\)`)
}

func (s *undertakerSuite) TestStateRemoveEnvironFails(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient, gc.NotNil)
	c.Assert(undertakerClient.RemoveModel(), gc.ErrorMatches, "can't remove model: model still alive")
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
	c.Assert(envInfo.Name, gc.Equals, "hosted-env")
	c.Assert(envInfo.GlobalName, gc.Equals, "user-admin/hosted-env")
	c.Assert(envInfo.IsSystem, jc.IsFalse)
	c.Assert(envInfo.Life, gc.Equals, life.Alive)
}

func (s *undertakerSuite) TestHostedProcessDyingEnviron(c *gc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	err := undertakerClient.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	factory.NewFactory(otherSt, s.StatePool).MakeApplication(c, nil)
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	err = otherSt.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
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
	c.Assert(err, gc.ErrorMatches, "can't remove model: model still alive")

	factory.NewFactory(otherSt, s.StatePool).MakeApplication(c, nil)
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)

	err = otherSt.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(undertakerClient.RemoveModel(), jc.ErrorIsNil)
	c.Assert(otherSt.EnsureModelRemoved(), jc.ErrorIsNil)
}

func (s *undertakerSuite) hostedAPI(c *gc.C) (*undertaker.Client, *state.State) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "hosted-env"})

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
	info.ModelTag = names.NewModelTag(otherState.ModelUUID())

	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)

	undertakerClient, err := undertaker.NewClient(otherAPIState, apiwatcher.NewNotifyWatcher)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(undertakerClient, gc.NotNil)

	return undertakerClient, otherState
}
