// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// NOTE: the testing of the general environment destruction code
// is found in apiserver/common/environdestroy_test.go.
//
// The tests here are around the validation and behaviour of
// the flags passed in to the destroy controller call.

type destroyControllerSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	controller *controller.ControllerAPI

	otherState     *state.State
	otherModel     *state.Model
	otherEnvOwner  names.UserTag
	otherModelUUID string
}

var _ = gc.Suite(&destroyControllerSuite{})

func (s *destroyControllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	controller, err := controller.NewControllerAPIv5(
		facadetest.Context{
			State_:     s.State,
			StatePool_: s.StatePool,
			Resources_: s.resources,
			Auth_:      s.authorizer,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	s.otherState = factory.NewFactory(s.State).MakeModel(c, &factory.ModelParams{
		Name:  "dummytoo",
		Owner: s.otherEnvOwner,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { s.otherState.Close() })
	s.otherModelUUID = s.otherState.ModelUUID()

	s.otherModel, err = s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyControllerSuite) TestDestroyControllerKillErrsOnHostedEnvsWithBlocks(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerReturnsBlockedEnvironmentsErr(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)

	_, err = s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyControllerSuite) TestDestroyControllerKillsHostedEnvs(c *gc.C) {
	err := s.controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerLeavesBlocksIfNotKillAll(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	numBlocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedEnvs(c *gc.C) {
	err := common.DestroyModel(common.NewModelManagerBackend(s.otherModel, s.StatePool), nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.controller.DestroyController(params.DestroyControllerArgs{})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerErrsOnNoHostedEnvsWithBlock(c *gc.C) {
	err := common.DestroyModel(common.NewModelManagerBackend(s.otherModel, s.StatePool), nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedEnvsWithBlockFail(c *gc.C) {
	err := common.DestroyModel(common.NewModelManagerBackend(s.otherModel, s.StatePool), nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(params.DestroyControllerArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageNotSpecified(c *gc.C) {
	f := factory.NewFactory(s.otherState)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	err := s.controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageSpecified(c *gc.C) {
	f := factory.NewFactory(s.otherState)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	destroyStorage := false
	err := s.controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageNotSpecifiedV3(c *gc.C) {
	controller, err := controller.NewControllerAPIv3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(s.otherState)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	err = controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageSpecifiedV3(c *gc.C) {
	controller, err := controller.NewControllerAPIv3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)

	destroyStorage := true
	err = controller.DestroyController(params.DestroyControllerArgs{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	c.Assert(err, gc.ErrorMatches, "destroy-storage unexpected on the v3 API")
}
