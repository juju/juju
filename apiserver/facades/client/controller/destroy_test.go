// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/facades/client/controller/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/blockcommand"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// NOTE: the testing of the general model destruction code
// is found in apiserver/common/modeldestroy_test.go.
//
// The tests here are around the validation and behaviour of
// the flags passed in to the destroy controller call.

type destroyControllerSuite struct {
	jujutesting.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	controller *controller.ControllerAPI

	otherState       *state.State
	otherModel       *state.Model
	otherModelOwner  names.UserTag
	otherModelUUID   string
	context          facadetest.MultiModelContext
	mockModelService *mocks.MockModelService
}

var _ = gc.Suite(&destroyControllerSuite{})

func (s *destroyControllerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.controller = s.controllerAPI(c)

	return ctrl
}

func (s *destroyControllerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	s.context = facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_:          s.ControllerModel(c).State(),
			StatePool_:      s.StatePool(),
			Resources_:      s.resources,
			Auth_:           s.authorizer,
			DomainServices_: s.ControllerDomainServices(c),
			Logger_:         loggertesting.WrapCheckLog(c),
		},
		DomainServicesForModel_: s.DefaultModelDomainServices(c),
	}

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

	var err error
	s.otherModel, err = s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
}

// controllerAPI sets up and returns a new instance of the controller API,
// It provides custom service getter functions and mock services
// to allow test-level control over their behavior.
func (s *destroyControllerSuite) controllerAPI(c *gc.C) *controller.ControllerAPI {
	stdCtx := context.Background()
	ctx := s.context
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		hub            = ctx.Hub()
		domainServices = ctx.DomainServices()
	)

	modelAgentServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Agent(), nil
	}
	modelConfigServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.ModelConfigService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Config(), nil
	}
	applicationServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.ApplicationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Application(), nil
	}
	statusServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.StatusService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Status(), nil
	}
	blockCommandServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.BlockCommandService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.BlockCommand(), nil
	}
	machineServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (model.MachineService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Machine(), nil
	}
	cloudSpecServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.ModelProviderService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.ModelProvider(), nil
	}

	api, err := controller.NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		hub,
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
		domainServices.Credential(),
		domainServices.Upgrade(),
		domainServices.Access(),
		machineServiceGetter,
		s.mockModelService,
		domainServices.ModelInfo(),
		domainServices.BlockCommand(),
		applicationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		cloudSpecServiceGetter,
		domainServices.Proxy(),
		func(c context.Context, modelUUID coremodel.UUID, legacyState facade.LegacyStateExporter) (controller.ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID, legacyState)
		},
		ctx.ObjectStore(),
		ctx.ControllerUUID(),
	)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *destroyControllerSuite) TestStub(c *gc.C) {
	// These will likely be tests for the service layer.
	c.Skip(`This suite is missing tests for the following scenarios:
- Successfully destroying a controller (life->dying) whose model has apps with storage when --destroy-storage is included.
- An error when destroying a controller also with storage, but without including --destroy-storage.
`)
}

func (s *destroyControllerSuite) TestDestroyControllerKillErrsOnHostedModelsWithBlocks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerReturnsBlockedModelErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.DefaultModelDomainServices(c).BlockCommand().GetBlocks(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerKillsHostedModels(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
		}, nil,
	)
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerLeavesBlocksIfNotKillAll(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")

	numBlocks, err := s.DefaultModelDomainServices(c).BlockCommand().GetBlocks(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModels(c *gc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)

	err := model.DestroyModel(
		context.Background(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), domainServices.ModelInfo(),
		nil, nil, nil, nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), jc.ErrorIsNil)
	c.Assert(s.otherModel.Life(), gc.Equals, state.Dying)
	c.Assert(s.otherModel.State().RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), jc.ErrorIs, errors.NotFound)

	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerErrsOnNoHostedModelsWithBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)
	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := model.DestroyModel(
		context.Background(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), domainServices.ModelInfo(),
		nil, nil, nil, nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in controller models")
	c.Assert(s.ControllerModel(c).Life(), gc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModelsWithBlockFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)

	err := model.DestroyModel(
		context.Background(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), domainServices.ModelInfo(),
		nil, nil, nil, nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err = s.controller.DestroyController(context.Background(), params.DestroyControllerArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := domainServices.BlockCommand().GetBlocks(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

// BlockAllChanges blocks all operations that could change the model.
func (s *destroyControllerSuite) BlockAllChanges(c *gc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(context.Background(), blockcommand.ChangeBlock, msg)
	c.Assert(err, jc.ErrorIsNil)
}

// BlockRemoveObject blocks all operations that remove
// machines, services, units or relations.
func (s *destroyControllerSuite) BlockRemoveObject(c *gc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, msg)
	c.Assert(err, jc.ErrorIsNil)
}

// BlockDestroyModel blocks destroy-model.
func (s *destroyControllerSuite) BlockDestroyModel(c *gc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(context.Background(), blockcommand.DestroyBlock, msg)
	c.Assert(err, jc.ErrorIsNil)
}

// AssertBlocked checks if given error is
// related to switched block.
func (s *destroyControllerSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}
