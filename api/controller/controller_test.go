// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type controllerSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *controllerSuite) OpenAPI(c *gc.C) *controller.Client {
	return controller.NewClient(s.APIState)
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: owner}).Close()

	sysManager := s.OpenAPI(c)
	envs, err := sysManager.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envs, gc.HasLen, 3)

	var obtained []string
	for _, env := range envs {
		obtained = append(obtained, fmt.Sprintf("%s/%s", env.Owner, env.Name))
	}
	expected := []string{
		"admin@local/dummymodel",
		"user@remote/first",
		"user@remote/second",
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	sysManager := s.OpenAPI(c)
	env, err := sysManager.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env["name"], gc.Equals, "dummymodel")
}

func (s *controllerSuite) TestDestroyController(c *gc.C) {
	s.Factory.MakeModel(c, &factory.ModelParams{Name: "foo"}).Close()

	sysManager := s.OpenAPI(c)
	err := sysManager.DestroyController(false)
	c.Assert(err, gc.ErrorMatches, "controller model cannot be destroyed before all other models are destroyed")
}

func (s *controllerSuite) TestListBlockedModels(c *gc.C) {
	err := s.State.SwitchBlockOn(state.ChangeBlock, "change block for controller")
	err = s.State.SwitchBlockOn(state.DestroyBlock, "destroy block for controller")
	c.Assert(err, jc.ErrorIsNil)

	sysManager := s.OpenAPI(c)
	results, err := sysManager.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []params.ModelBlockInfo{
		params.ModelBlockInfo{
			Name:     "dummymodel",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})
}

func (s *controllerSuite) TestRemoveBlocks(c *gc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	sysManager := s.OpenAPI(c)
	err := sysManager.RemoveBlocks()
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *controllerSuite) TestWatchAllModels(c *gc.C) {
	// The WatchAllModels infrastructure is comprehensively tested
	// else. This test just ensure that the API calls work end-to-end.
	sysManager := s.OpenAPI(c)

	w, err := sysManager.WatchAllModels()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := w.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	deltasC := make(chan []multiwatcher.Delta)
	go func() {
		deltas, err := w.Next()
		c.Assert(err, jc.ErrorIsNil)
		deltasC <- deltas
	}()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, gc.HasLen, 1)
		modelInfo := deltas[0].Entity.(*multiwatcher.ModelInfo)

		env, err := s.State.Model()
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(modelInfo.ModelUUID, gc.Equals, env.UUID())
		c.Assert(modelInfo.Name, gc.Equals, env.Name())
		c.Assert(modelInfo.Life, gc.Equals, multiwatcher.Life("alive"))
		c.Assert(modelInfo.Owner, gc.Equals, env.Owner().Id())
		c.Assert(modelInfo.ServerUUID, gc.Equals, env.ControllerUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	controller := s.OpenAPI(c)
	modelTag := s.State.ModelTag()
	results, err := controller.ModelStatus(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []base.ModelStatus{{
		UUID:               modelTag.Id(),
		HostedMachineCount: 0,
		ServiceCount:       0,
		Owner:              "admin@local",
		Life:               params.Alive,
	}})
}
