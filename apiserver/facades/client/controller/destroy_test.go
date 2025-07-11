// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/model"
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

	otherState           *state.State
	otherModel           *state.Model
	otherModelOwner      names.UserTag
	otherModelUUID       string
	context              facadetest.MultiModelContext
	mockModelService     *mocks.MockModelService
	mockModelInfoService *mocks.MockModelInfoService
}

func TestDestroyControllerSuite(t *stdtesting.T) {
	tc.Run(t, &destroyControllerSuite{})
}

func (s *destroyControllerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.mockModelInfoService = mocks.NewMockModelInfoService(ctrl)
	s.controller = s.controllerAPI(c)

	return ctrl
}

func (s *destroyControllerSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

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
		UUID: s.DefaultModelUUID,
	})
	s.AddCleanup(func(c *tc.C) { s.otherState.Close() })
	s.otherModelUUID = s.DefaultModelUUID.String()

	var err error
	s.otherModel, err = s.otherState.Model()
	c.Assert(err, tc.ErrorIsNil)
}

// controllerAPI sets up and returns a new instance of the controller API,
// It provides custom service getter functions and mock services
// to allow test-level control over their behavior.
func (s *destroyControllerSuite) controllerAPI(c *tc.C) *controller.ControllerAPI {
	stdCtx := c.Context()
	ctx := s.context
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		domainServices = ctx.DomainServices()
	)

	credentialServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.CredentialService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Credential(), nil
	}
	upgradeServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.UpgradeService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Upgrade(), nil
	}
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
	relationServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.RelationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Relation(), nil
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
	machineServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.MachineService, error) {
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
	modelMigrationServiceGetter := func(c context.Context, modelUUID coremodel.UUID) (controller.ModelMigrationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.ModelMigration(), nil
	}

	api, err := controller.NewControllerAPI(
		stdCtx,
		st,
		pool,
		authorizer,
		resources,
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Access(),
		s.mockModelService,
		s.mockModelInfoService,
		domainServices.BlockCommand(),
		modelMigrationServiceGetter,
		credentialServiceGetter,
		upgradeServiceGetter,
		applicationServiceGetter,
		relationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		cloudSpecServiceGetter,
		machineServiceGetter,
		domainServices.Proxy(),
		func(c context.Context, modelUUID coremodel.UUID) (controller.ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID)
		},
		ctx.ObjectStore(),
		ctx.ControllerModelUUID(),
		ctx.ControllerUUID(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *destroyControllerSuite) TestStub(c *tc.C) {
	// These will likely be tests for the service layer.
	c.Skip(`This suite is missing tests for the following scenarios:
- Successfully destroying a controller (life->dying) whose model has apps with storage when --destroy-storage is included.
- An error when destroying a controller also with storage, but without including --destroy-storage.
`)
}

func (s *destroyControllerSuite) TestDestroyControllerKillErrsOnHostedModelsWithBlocks(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, tc.ErrorMatches, "found blocks in controller models")

	c.Assert(s.ControllerModel(c).Life(), tc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerReturnsBlockedModelErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)

	numBlocks, err := s.DefaultModelDomainServices(c).BlockCommand().GetBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(numBlocks), tc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerKillsHostedModels(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
		}, nil,
	)
	s.mockModelInfoService.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)
	s.mockModelInfoService.EXPECT().HasValidCredential(gomock.Any()).Return(true, nil)

	err := s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{
		DestroyModels: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), tc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerLeavesBlocksIfNotKillAll(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err := s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{})
	c.Assert(err, tc.ErrorMatches, "found blocks in controller models")

	numBlocks, err := s.DefaultModelDomainServices(c).BlockCommand().GetBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(numBlocks), tc.Equals, 2)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModels(c *tc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)

	err := model.DestroyModel(
		c.Context(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), domainServices.ModelInfo(),
		nil, nil, nil, nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), tc.ErrorIsNil)
	c.Assert(s.otherModel.Life(), tc.Equals, state.Dying)
	c.Assert(s.otherModel.State().RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(s.otherModel.Refresh(), tc.ErrorIs, errors.NotFound)

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	s.mockModelInfoService.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)
	s.mockModelInfoService.EXPECT().HasValidCredential(gomock.Any()).Return(true, nil)
	err = s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.ControllerModel(c).Life(), tc.Equals, state.Dying)
}

func (s *destroyControllerSuite) TestDestroyControllerErrsOnNoHostedModelsWithBlock(c *tc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)
	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	s.mockModelInfoService.EXPECT().HasValidCredential(gomock.Any()).Return(true, nil)

	err := model.DestroyModel(
		c.Context(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), s.mockModelInfoService,
		nil, nil, nil, nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{})
	c.Assert(err, tc.ErrorMatches, "found blocks in controller models")
	c.Assert(s.ControllerModel(c).Life(), tc.Equals, state.Alive)
}

func (s *destroyControllerSuite) TestDestroyControllerNoHostedModelsWithBlockFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	domainServices := s.DefaultModelDomainServices(c)

	err := model.DestroyModel(
		c.Context(), model.NewModelManagerBackend(s.otherModel, s.StatePool()),
		domainServices.BlockCommand(), domainServices.ModelInfo(),
		nil, nil, nil, nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyModel")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			coremodel.UUID(s.ControllerUUID),
			coremodel.UUID(s.otherModelUUID),
		}, nil,
	)
	err = s.controller.DestroyController(c.Context(), params.DestroyControllerArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)

	numBlocks, err := domainServices.BlockCommand().GetBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(numBlocks), tc.Equals, 2)
}

// BlockAllChanges blocks all operations that could change the model.
func (s *destroyControllerSuite) BlockAllChanges(c *tc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(c.Context(), blockcommand.ChangeBlock, msg)
	c.Assert(err, tc.ErrorIsNil)
}

// BlockRemoveObject blocks all operations that remove
// machines, services, units or relations.
func (s *destroyControllerSuite) BlockRemoveObject(c *tc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(c.Context(), blockcommand.RemoveBlock, msg)
	c.Assert(err, tc.ErrorIsNil)
}

// BlockDestroyModel blocks destroy-model.
func (s *destroyControllerSuite) BlockDestroyModel(c *tc.C, msg string) {
	err := s.DefaultModelDomainServices(c).BlockCommand().SwitchBlockOn(c.Context(), blockcommand.DestroyBlock, msg)
	c.Assert(err, tc.ErrorIsNil)
}

// AssertBlocked checks if given error is
// related to switched block.
func (s *destroyControllerSuite) AssertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}
