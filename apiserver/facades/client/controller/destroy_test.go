// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	domainservicestesting "github.com/juju/juju/domain/services/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

// NOTE: the testing of the general model destruction code
// is found in apiserver/common/modeldestroy_test.go.
//
// The tests here are around the validation and behaviour of
// the flags passed in to the destroy controller call.

type destroyControllerSuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	controller *controller.ControllerAPI

	otherState      *state.State
	otherModel      *state.Model
	otherModelOwner names.UserTag
	otherModelUUID  string
}

var _ = gc.Suite(&destroyControllerSuite{})

func (s *destroyControllerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.BlockHelper = commontesting.NewBlockHelper(s.OpenControllerModelAPI(c))
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	testController, err := controller.LatestAPI(
		context.Background(),
		facadetest.MultiModelContext{
			ModelContext: facadetest.ModelContext{
				State_:          s.ControllerModel(c).State(),
				StatePool_:      s.StatePool(),
				Resources_:      s.resources,
				Auth_:           s.authorizer,
				DomainServices_: domainservicestesting.NewTestingDomainServices(),
				Logger_:         loggertesting.WrapCheckLog(c),
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = testController

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.otherModelOwner = names.NewUserTag("jess@dummy")
	s.otherState = f.MakeModel(c, &factory.ModelParams{
		Name:  "dummytoo",
		Owner: s.otherModelOwner,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { s.otherState.Close() })
	s.otherModelUUID = s.otherState.ModelUUID()

	s.otherModel, err = s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyControllerSuite) TestDestroyControllerKillErrsOnHostedModelsWithBlocks(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerReturnsBlockedModelErr(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.ControllerModel(c).State().AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)

	_, err = s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyControllerSuite) TestDestroyControllerKillsHostedModels(c *gc.C) {
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerLeavesBlocksIfNotKillAll(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	numBlocks, err := s.ControllerModel(c).State().AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModels(c *gc.C) {
	err := common.DestroyModel(context.Background(), common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.otherModel, s.StatePool()), nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), jc.ErrorIsNil)
	c.Assert(s.otherModel.Life(), gc.Equals, state.Dying)
	c.Assert(s.otherModel.State().RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), jc.ErrorIs, errors.NotFound)

	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerErrsOnNoHostedModelsWithBlock(c *gc.C) {
	err := common.DestroyModel(context.Background(), common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.otherModel, s.StatePool()), nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")
	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModelsWithBlockFail(c *gc.C) {
	err := common.DestroyModel(context.Background(), common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.otherModel, s.StatePool()), nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.ControllerModel(c).State().AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageNotSpecified(c *gc.C) {
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	modelState := f.MakeModel(c, &factory.ModelParams{
		UUID:  &modelUUID,
		Name:  "modelconfig",
		Owner: s.otherModelOwner,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { _ = modelState.Close() })

	f2 := factory.NewFactory(modelState, s.StatePool(), controllerConfig).
		WithModelConfigService(s.ControllerDomainServices(c).Config())
	f2.MakeUnit(c, &factory.UnitParams{
		Application: f2.MakeApplication(c, &factory.ApplicationParams{
			Charm: f2.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.ErrorIs, stateerrors.PersistentStorageError)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerDestroyStorageSpecified(c *gc.C) {
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	modelState := f.MakeModel(c, &factory.ModelParams{
		UUID:  &modelUUID,
		Name:  "modelconfig",
		Owner: s.otherModelOwner,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { _ = modelState.Close() })

	f2 := factory.NewFactory(modelState, s.StatePool(), controllerConfig).
		WithModelConfigService(s.ControllerDomainServices(c).Config())
	f2.MakeUnit(c, &factory.UnitParams{
		Application: f2.MakeApplication(c, &factory.ApplicationParams{
			Charm: f2.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	destroyStorage := false
	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerForce(c *gc.C) {
	force := true
	timeout := 1 * time.Hour
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
		Force:         &force,
		ModelTimeout:  &timeout,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.ControllerModel(c).ForceDestroyed(), jc.IsTrue)
	c.Assert(s.ControllerModel(c).DestroyTimeout().Hours(), gc.Equals, 1.0)
}
