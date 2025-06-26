// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/facades/client/controller/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	modelerrors "github.com/juju/juju/domain/model/errors"
	servicefactorytesting "github.com/juju/juju/domain/services/testing"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/docker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalservices "github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type controllerSuite struct {
	statetesting.StateSuite
	servicefactorytesting.DomainServicesSuite

	controllerConfigAttrs map[string]any

	controller       *controller.ControllerAPI
	resources        *common.Resources
	watcherRegistry  facade.WatcherRegistry
	authorizer       apiservertesting.FakeAuthorizer
	context          facadetest.MultiModelContext
	leadershipReader leadership.Reader
	mockModelService *mocks.MockModelService
}

func TestControllerSuite(t *stdtesting.T) {
	tc.Run(t, &controllerSuite{})
}

func (s *controllerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.controller = s.controllerAPI(c)

	return ctrl
}

func (s *controllerSuite) SetUpSuite(c *tc.C) {
	s.StateSuite.SetUpSuite(c)
	s.DomainServicesSuite.SetUpSuite(c)
}

func (s *controllerSuite) TearDownSuite(c *tc.C) {
	s.DomainServicesSuite.TearDownSuite(c)
	s.StateSuite.TearDownSuite(c)
}

func (s *controllerSuite) SetUpTest(c *tc.C) {
	if s.controllerConfigAttrs == nil {
		s.controllerConfigAttrs = map[string]any{}
	}
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})
	controllerCfg := testing.FakeControllerConfig()
	for key, value := range s.controllerConfigAttrs {
		controllerCfg[key] = value
	}

	s.StateSuite.ControllerConfig = controllerCfg
	s.StateSuite.SetUpTest(c)
	s.DomainServicesSuite.ControllerConfig = controllerCfg
	s.DomainServicesSuite.SetUpTest(c)

	domainServiceGetter := s.DomainServicesGetter(c, s.NoopObjectStore(c), s.NoopLeaseManager(c))
	jujujujutesting.SeedDatabase(c, s.ControllerSuite.TxnRunner(), domainServiceGetter(s.ControllerModelUUID), controllerCfg)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	s.leadershipReader = noopLeadershipReader{}
	s.context = facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_:            s.State,
			StatePool_:        s.StatePool,
			Resources_:        s.resources,
			WatcherRegistry_:  s.watcherRegistry,
			Auth_:             s.authorizer,
			DomainServices_:   s.ControllerDomainServices(c),
			Logger_:           loggertesting.WrapCheckLog(c),
			LeadershipReader_: s.leadershipReader,
			ControllerUUID_:   modeltesting.GenModelUUID(c).String(),
			ModelUUID_:        modeltesting.GenModelUUID(c),
		},
		DomainServicesForModelFunc_: func(modelUUID model.UUID) internalservices.DomainServices {
			return s.ModelDomainServices(c, modelUUID)
		},
	}

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *controllerSuite) TearDownTest(c *tc.C) {
	s.DomainServicesSuite.TearDownTest(c)
	s.StateSuite.TearDownTest(c)
}

// controllerAPI sets up and returns a new instance of the controller API,
// It provides custom service getter functions and mock services
// to allow test-level control over their behavior.
func (s *controllerSuite) controllerAPI(c *tc.C) *controller.ControllerAPI {
	stdCtx := c.Context()
	ctx := s.context
	var (
		st             = ctx.State()
		authorizer     = ctx.Auth()
		pool           = ctx.StatePool()
		resources      = ctx.Resources()
		domainServices = ctx.DomainServices()
	)

	credentialServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.CredentialService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Credential(), nil
	}
	upgradeServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.UpgradeService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Upgrade(), nil
	}
	modelAgentServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.ModelAgentService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Agent(), nil
	}
	modelConfigServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.ModelConfigService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Config(), nil
	}
	applicationServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.ApplicationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Application(), nil
	}
	relationServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.RelationService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Relation(), nil
	}
	statusServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.StatusService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Status(), nil
	}
	blockCommandServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.BlockCommandService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.BlockCommand(), nil
	}
	machineServiceGetter := func(c context.Context, modelUUID model.UUID) (commonmodel.MachineService, error) {
		svc, err := ctx.DomainServicesForModel(c, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Machine(), nil
	}
	cloudSpecServiceGetter := func(c context.Context, modelUUID model.UUID) (controller.ModelProviderService, error) {
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
		ctx.Logger().Child("controller"),
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Access(),
		machineServiceGetter,
		s.mockModelService,
		domainServices.ModelInfo(),
		domainServices.BlockCommand(),
		credentialServiceGetter,
		upgradeServiceGetter,
		applicationServiceGetter,
		relationServiceGetter,
		statusServiceGetter,
		modelAgentServiceGetter,
		modelConfigServiceGetter,
		blockCommandServiceGetter,
		cloudSpecServiceGetter,
		domainServices.Proxy(),
		func(c context.Context, modelUUID model.UUID, legacyState facade.LegacyStateExporter) (controller.ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID, legacyState)
		},
		ctx.ObjectStore(),
		ctx.ControllerModelUUID(),
		ctx.ControllerUUID(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *controllerSuite) TestNewAPIRefusesNonClient(c *tc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := controller.LatestAPI(c.Context(), facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_:          s.State,
			Resources_:      s.resources,
			Auth_:           anAuthoriser,
			DomainServices_: s.ControllerDomainServices(c),
			Logger_:         loggertesting.WrapCheckLog(c),
		},
	})
	c.Assert(endPoint, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestHostedModelConfigs_OnlyHostedModelsReturned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelService.EXPECT().ListAllModels(gomock.Any()).Return(
		[]model.Model{
			{
				Name:      "first",
				Qualifier: "prod",
				UUID:      modeltesting.GenModelUUID(c),
			},
			{
				Name:      "second",
				Qualifier: "staging",
				UUID:      modeltesting.GenModelUUID(c),
			},
		}, nil,
	)
	s.mockModelService.EXPECT().ControllerModel(gomock.Any()).Return(
		model.Model{
			Name:      "controller",
			Qualifier: "prod",
			UUID:      s.ControllerModelUUID,
		}, nil,
	)
	results, err := s.controller.HostedModelConfigs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results.Models), tc.Equals, 2)

	one := results.Models[0]
	two := results.Models[1]

	c.Assert(one.Name, tc.Equals, "first")
	c.Assert(one.Qualifier, tc.Equals, "prod")
	c.Assert(two.Name, tc.Equals, "second")
	c.Assert(two.Qualifier, tc.Equals, "staging")
}

func (s *controllerSuite) makeCloudSpec(c *tc.C, pSpec *params.CloudSpec) environscloudspec.CloudSpec {
	c.Assert(pSpec, tc.NotNil)
	var credential *cloud.Credential
	if pSpec.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(pSpec.Credential.AuthType),
			pSpec.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := environscloudspec.CloudSpec{
		Type:             pSpec.Type,
		Name:             pSpec.Name,
		Region:           pSpec.Region,
		Endpoint:         pSpec.Endpoint,
		IdentityEndpoint: pSpec.IdentityEndpoint,
		StorageEndpoint:  pSpec.StorageEndpoint,
		Credential:       credential,
	}
	c.Assert(spec.Validate(), tc.ErrorIsNil)
	return spec
}

func (s *controllerSuite) TestHostedModelConfigs_CanOpenEnviron(c *tc.C) {
	defer s.setupMocks(c).Finish()
	c.Skip("Hosted model config is skipped because the tests aren't wired up correctly")
	owner := names.NewUserTag("owner")
	st1 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner})
	defer func() { _ = st1.Close() }()
	remoteUserTag := names.NewUserTag("user").WithDomain("remote")
	st2 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: remoteUserTag})
	defer func() { _ = st2.Close() }()

	results, err := s.controller.HostedModelConfigs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results.Models), tc.Equals, 2)

	for _, model := range results.Models {
		c.Assert(model.Error, tc.IsNil)

		cfg, err := config.New(config.NoDefaults, model.Config)
		c.Assert(err, tc.ErrorIsNil)
		spec := s.makeCloudSpec(c, model.CloudSpec)
		_, err = environs.New(c.Context(), environs.OpenParams{
			Cloud:  spec,
			Config: cfg,
		}, environs.NoopCredentialInvalidator())
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *controllerSuite) TestListBlockedModels(c *tc.C) {
	defer s.setupMocks(c).Finish()
	otherDomainServices := s.DefaultModelDomainServices(c)
	otherBlockCommands := otherDomainServices.BlockCommand()
	err := otherBlockCommands.SwitchBlockOn(c.Context(), blockcommand.ChangeBlock, "ChangeBlock")
	c.Assert(err, tc.ErrorIsNil)
	err = otherBlockCommands.SwitchBlockOn(c.Context(), blockcommand.DestroyBlock, "DestroyBlock")
	c.Assert(err, tc.ErrorIsNil)
	models := []model.Model{
		{
			UUID:      s.DomainServicesSuite.DefaultModelUUID,
			Name:      "test",
			Qualifier: "prod",
			ModelType: model.IAAS,
		},
	}
	s.mockModelService.EXPECT().ListAllModels(gomock.Any()).Return(
		models, nil,
	)

	list, err := s.controller.ListBlockedModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(list.Models, tc.DeepEquals, []params.ModelBlockInfo{
		{
			UUID:      s.DomainServicesSuite.DefaultModelUUID.String(),
			Name:      "test",
			Qualifier: "prod",
			Blocks: []string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})

}

func (s *controllerSuite) TestListBlockedModelsNoBlocks(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockModelService.EXPECT().ListAllModels(gomock.Any()).Return(
		nil, nil,
	)
	list, err := s.controller.ListBlockedModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(list.Models, tc.HasLen, 0)
}

func (s *controllerSuite) TestControllerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg, err := s.controller.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	cfgFromDB, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], tc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], tc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], tc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestControllerConfigFromNonController(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer func() { _ = st.Close() }()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.Owner}
	controller, err := controller.LatestAPI(
		c.Context(),
		facadetest.MultiModelContext{
			ModelContext: facadetest.ModelContext{
				State_:          st,
				Resources_:      common.NewResources(),
				Auth_:           authorizer,
				DomainServices_: s.ControllerDomainServices(c),
				Logger_:         loggertesting.WrapCheckLog(c),
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	cfg, err := controller.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	cfgFromDB, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], tc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], tc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], tc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestRemoveBlocks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	otherDomainServices := s.ModelDomainServices(c, s.DefaultModelUUID)
	otherBlockCommands := otherDomainServices.BlockCommand()
	otherBlockCommands.SwitchBlockOn(c.Context(), blockcommand.ChangeBlock, "TestChangeBlock")
	otherBlockCommands.SwitchBlockOn(c.Context(), blockcommand.DestroyBlock, "TestChangeBlock")

	otherBlocks, err := otherBlockCommands.GetBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(otherBlocks, tc.HasLen, 2)

	s.mockModelService.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]model.UUID{
			s.DefaultModelUUID,
		}, nil,
	)
	err = s.controller.RemoveBlocks(c.Context(), params.RemoveBlocksArgs{All: true})
	c.Assert(err, tc.ErrorIsNil)

	otherBlocks, err = otherBlockCommands.GetBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(otherBlocks, tc.HasLen, 0)
}

func (s *controllerSuite) TestRemoveBlocksNotAll(c *tc.C) {
	defer s.setupMocks(c).Finish()
	err := s.controller.RemoveBlocks(c.Context(), params.RemoveBlocksArgs{})
	c.Assert(err, tc.ErrorMatches, "not supported")
}

func (s *controllerSuite) TestInitiateMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Create two hosted models to migrate.
	st1 := s.Factory.MakeModel(c, nil)
	defer func() { _ = st1.Close() }()
	model1, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(model1.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(model1.UUID()),
			Name:      model1.Name(),
			Qualifier: model.QualifierFromUserTag(model1.Owner()),
		}, nil,
	)

	st2 := s.Factory.MakeModel(c, nil)
	defer func() { _ = st2.Close() }()
	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(model2.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(model2.UUID()),
			Name:      model2.Name(),
			Qualifier: model.QualifierFromUserTag(model2.Owner()),
		}, nil,
	)

	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	macsJSON, err := json.Marshal([]macaroon.Slice{{mac}})
	c.Assert(err, tc.ErrorIsNil)

	controller.SetPrecheckResult(s, nil)

	// Kick off migrations
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: model1.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag:   randomControllerTag(),
					ControllerAlias: "", // intentionally left empty; simulates older client
					Addrs:           []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:          "cert1",
					AuthTag:         names.NewUserTag("admin1").String(),
					Password:        "secret1",
					Token:           "token1",
				},
			}, {
				ModelTag: model2.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag:   randomControllerTag(),
					ControllerAlias: "target-controller",
					Addrs:           []string{"3.3.3.3:3333"},
					CACert:          "cert2",
					AuthTag:         names.NewUserTag("admin2").String(),
					Macaroons:       string(macsJSON),
					Password:        "secret2",
					Token:           "token2",
				},
			},
		},
	}

	out, err := s.controller.InitiateMigration(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Results, tc.HasLen, 2)

	states := []*state.State{st1, st2}
	for i, spec := range args.Specs {
		c.Log(i)
		st := states[i]
		result := out.Results[i]

		c.Assert(result.Error, tc.IsNil)
		c.Check(result.ModelTag, tc.Equals, spec.ModelTag)

		// Ensure the migration made it into the DB correctly.
		mig, err := st.LatestMigration()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(mig.InitiatedBy(), tc.Equals, s.Owner.Id())

		targetInfo, err := mig.TargetInfo()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(targetInfo.ControllerTag.String(), tc.Equals, spec.TargetInfo.ControllerTag)
		c.Check(targetInfo.ControllerAlias, tc.Equals, spec.TargetInfo.ControllerAlias)
		c.Check(targetInfo.Addrs, tc.SameContents, spec.TargetInfo.Addrs)
		c.Check(targetInfo.CACert, tc.Equals, spec.TargetInfo.CACert)
		c.Check(targetInfo.AuthTag.String(), tc.Equals, spec.TargetInfo.AuthTag)
		c.Check(targetInfo.Password, tc.Equals, spec.TargetInfo.Password)
		c.Check(targetInfo.Token, tc.Equals, spec.TargetInfo.Token)

		if spec.TargetInfo.Macaroons != "" {
			macJSONdb, err := json.Marshal(targetInfo.Macaroons)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(string(macJSONdb), tc.Equals, spec.TargetInfo.Macaroons)
		}
	}
}

func (s *controllerSuite) TestInitiateMigrationSpecError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Create a hosted model to migrate.
	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Kick off the migration with missing details.
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: m.ModelTag().String(),
			// TargetInfo missing
		}},
	}

	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(m.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(m.UUID()),
			Name:      m.Name(),
			Qualifier: model.QualifierFromUserTag(m.Owner()),
		}, nil,
	)
	out, err := s.controller.InitiateMigration(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Results, tc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, tc.Equals, args.Specs[0].ModelTag)
	c.Check(result.MigrationId, tc.Equals, "")
	c.Check(result.Error, tc.ErrorMatches, "controller tag: .+ is not a valid tag")
}

func (s *controllerSuite) TestInitiateMigrationPartialFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()
	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	controller.SetPrecheckResult(s, nil)

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(m.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(m.UUID()),
			Name:      m.Name(),
			Qualifier: model.QualifierFromUserTag(m.Owner()),
		}, nil,
	)

	randomUUID := modeltesting.GenModelUUID(c)
	randomModelTag := names.NewModelTag(randomUUID.String())

	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(randomModelTag.Id())).Return(
		model.Model{}, modelerrors.NotFound,
	)

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: m.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
				},
			}, {
				ModelTag: randomModelTag.String(), // Doesn't exist.
			},
		},
	}
	out, err := s.controller.InitiateMigration(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Results, tc.HasLen, 2)

	c.Check(out.Results[0].ModelTag, tc.Equals, m.ModelTag().String())
	c.Check(out.Results[0].Error, tc.IsNil)

	c.Check(out.Results[1].ModelTag, tc.Equals, args.Specs[1].ModelTag)
	c.Check(out.Results[1].Error.Error(), tc.Equals, fmt.Sprintf("model %q not found", randomModelTag.Id()))
}

func (s *controllerSuite) TestInitiateMigrationInvalidMacaroons(c *tc.C) {
	defer s.setupMocks(c).Finish()
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: m.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Macaroons:     "BLAH",
				},
			},
		},
	}
	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(m.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(m.UUID()),
			Name:      m.Name(),
			Qualifier: model.QualifierFromUserTag(m.Owner()),
		}, nil,
	)
	out, err := s.controller.InitiateMigration(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Results, tc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, tc.Equals, args.Specs[0].ModelTag)
	c.Check(result.Error, tc.ErrorMatches, "invalid macaroons: .+")
}

func (s *controllerSuite) TestInitiateMigrationPrecheckFail(c *tc.C) {
	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	defer s.setupMocks(c).Finish()
	st := s.Factory.MakeModel(c, &factory.ModelParams{UUID: s.DefaultModelUUID})
	defer st.Close()

	controller.SetPrecheckResult(s, errors.New("boom"))

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelService.EXPECT().Model(gomock.Any(), model.UUID(m.ModelTag().Id())).Return(
		model.Model{
			UUID:      model.UUID(m.UUID()),
			Name:      m.Name(),
			Qualifier: model.Qualifier(m.Owner().Id()),
		}, nil,
	)

	c.Assert(err, tc.ErrorIsNil)

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: m.ModelTag().String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: randomControllerTag(),
				Addrs:         []string{"1.1.1.1:1111"},
				CACert:        "cert1",
				AuthTag:       names.NewUserTag("admin1").String(),
				Password:      "secret1",
			},
		}},
	}
	out, err := s.controller.InitiateMigration(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out.Results, tc.HasLen, 1)
	c.Check(out.Results[0].Error, tc.ErrorMatches, "boom")

	active, err := st.IsMigrationActive()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(active, tc.IsFalse)
}

func randomControllerTag() string {
	uuid := uuid.MustNewUUID().String()
	return names.NewControllerTag(uuid).String()
}

func (s *controllerSuite) TestGrantControllerInvalidUserTag(c *tc.C) {
	defer s.setupMocks(c).Finish()
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user@local",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify controller access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyControllerAccessRequest{
			Changes: []params.ModifyControllerAccess{{
				UserTag: testParam.tag,
				Action:  params.GrantControllerAccess,
				Access:  string(permission.SuperuserAccess),
			}}}

		result, err := s.controller.ModifyControllerAccess(c.Context(), args)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
	}
}

func (s *controllerSuite) TestModelStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Check that we don't err out immediately if a model errs.
	results, err := s.controller.ModelStatus(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: s.Model.ModelTag().String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.controller.ModelStatus(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[1].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we return successfully if no errors.
	results, err = s.controller.ModelStatus(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
}

func (s *controllerSuite) TestConfigSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	config, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// Sanity check.
	c.Assert(config.AuditingEnabled(), tc.Equals, false)
	c.Assert(config.SSHServerPort(), tc.Equals, 17022)

	err = s.controller.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"auditing-enabled": true,
	}})
	c.Assert(err, tc.ErrorIsNil)

	config, err = controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config.AuditingEnabled(), tc.Equals, true)
}

func (s *controllerSuite) TestConfigSetRequiresSuperUser(c *tc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("username"),
	}
	endpoint, err := controller.LatestAPI(
		c.Context(),
		facadetest.MultiModelContext{
			ModelContext: facadetest.ModelContext{
				State_:          s.State,
				Resources_:      s.resources,
				Auth_:           anAuthoriser,
				DomainServices_: s.ControllerDomainServices(c),
				Logger_:         loggertesting.WrapCheckLog(c),
			},
		})
	c.Assert(err, tc.ErrorIsNil)

	err = endpoint.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"something": 23,
	}})

	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestConfigSetCAASImageRepo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// TODO(dqlite): move this test when ConfigSet CAASImageRepo logic moves.
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	config, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config.CAASImageRepo(), tc.Equals, "")

	err = s.controller.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": "juju-repo.local",
	}})
	c.Assert(err, tc.ErrorMatches, `cannot change caas-image-repo as it is not currently set`)

	err = controllerConfigService.UpdateControllerConfig(
		c.Context(),
		map[string]interface{}{
			"caas-image-repo": "jujusolutions",
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.controller.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": "juju-repo.local",
	}})
	c.Assert(err, tc.ErrorMatches, `cannot change caas-image-repo: repository read-only, only authentication can be updated`)

	err = s.controller.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": `{"repository":"jujusolutions","username":"foo","password":"bar"}`,
	}})
	c.Assert(err, tc.ErrorMatches, `cannot change caas-image-repo: unable to add authentication details`)

	err = controllerConfigService.UpdateControllerConfig(
		c.Context(),
		map[string]interface{}{
			"caas-image-repo": `{"repository":"jujusolutions","username":"bar","password":"foo"}`,
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.controller.ConfigSet(c.Context(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": `{"repository":"jujusolutions","username":"foo","password":"bar"}`,
	}})
	c.Assert(err, tc.ErrorIsNil)

	config, err = controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	repoDetails, err := docker.NewImageRepoDetails(config.CAASImageRepo())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(repoDetails, tc.DeepEquals, docker.ImageRepoDetails{
		Repository: "jujusolutions",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "foo",
			Password: "bar",
		},
	})
}

func (s *controllerSuite) TestMongoVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()
	result, err := s.controller.MongoVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var resErr *params.Error
	c.Assert(result.Error, tc.Equals, resErr)
	// We can't guarantee which version of mongo is running, so let's just
	// attempt to match it to a very basic version (major.minor.patch)
	c.Assert(result.Result, tc.Matches, "^([0-9]{1,}).([0-9]{1,}).([0-9]{1,})$")
}

func (s *controllerSuite) TestIdentityProviderURL(c *tc.C) {
	c.Skip("FIXME: This test calls setup test and tear down test... it should not.")
	// Preserve default controller config as we will be mutating it just
	// for this test
	defer func(orig map[string]interface{}) {
		s.controllerConfigAttrs = orig
	}(s.controllerConfigAttrs)

	ctrl := s.setupMocks(c)
	// Our default test configuration does not specify an IdentityURL
	urlRes, err := s.controller.IdentityProviderURL(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(urlRes.Result, tc.Equals, "")
	ctrl.Finish()

	// IdentityURL cannot be changed after bootstrap; we need to spin up
	// another controller with IdentityURL pre-configured
	s.TearDownTest(c)

	expURL := "https://api.jujucharms.com/identity"
	s.controllerConfigAttrs = map[string]any{
		corecontroller.IdentityURL: expURL,
	}

	s.SetUpTest(c)
	ctrl = s.setupMocks(c)
	defer ctrl.Finish()

	urlRes, err = s.controller.IdentityProviderURL(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(urlRes.Result, tc.Equals, expURL)
}

func (s *controllerSuite) newSummaryWatcherFacade(c *tc.C, id string) *apiserver.SrvModelSummaryWatcher {
	context := s.context
	context.ID_ = id
	watcher, err := apiserver.NewModelSummaryWatcher(context)
	c.Assert(err, tc.ErrorIsNil)
	return watcher
}

func (s *controllerSuite) TestWatchAllModelSummariesByAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// TODO(dqlite) - implement me
	c.Skip("watch model summaries to be implemented")
	// Default authorizer is an admin.
	result, err := s.controller.WatchAllModelSummaries(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	watcherAPI := s.newSummaryWatcherFacade(c, result.WatcherID)

	resultC := make(chan params.SummaryWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		c.Assert(result, tc.DeepEquals, params.SummaryWatcherNextResults{
			Models: []params.ModelAbstract{
				{
					UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					Controller: "", // TODO(thumper): add controller name next branch
					Name:       "controller",
					Admins:     []string{"test-admin"},
					Cloud:      "dummy",
					Region:     "dummy-region",
					Status:     "green",
					Messages:   []params.ModelSummaryMessage{},
				}}})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestWatchAllModelSummariesByNonAdmin(c *tc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewLocalUserTag("bob"),
	}
	endPoint, err := controller.LatestAPI(
		c.Context(),
		facadetest.MultiModelContext{
			ModelContext: facadetest.ModelContext{
				State_:          s.State,
				Resources_:      s.resources,
				Auth_:           anAuthoriser,
				DomainServices_: s.ControllerDomainServices(c),
				Logger_:         loggertesting.WrapCheckLog(c),
			},
		})
	c.Assert(err, tc.ErrorIsNil)

	_, err = endPoint.WatchAllModelSummaries(c.Context())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestWatchModelSummariesByNonAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// TODO(dqlite) - implement me
	c.Skip("watch model summaries to be implemented")

	// Default authorizer is an admin. As a user, admin can't see
	// Bob's model.
	result, err := s.controller.WatchModelSummaries(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	watcherAPI := s.newSummaryWatcherFacade(c, result.WatcherID)

	resultC := make(chan params.SummaryWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		c.Assert(result, tc.DeepEquals, params.SummaryWatcherNextResults{
			Models: []params.ModelAbstract{
				{
					UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					Controller: "", // TODO(thumper): add controller name next branch
					Name:       "controller",
					Admins:     []string{"test-admin"},
					Cloud:      "dummy",
					Region:     "dummy-region",
					Status:     "green",
					Messages:   []params.ModelSummaryMessage{},
				}}})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

}

type accessSuite struct {
	statetesting.StateSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	accessService       *mocks.MockControllerAccessService
	modelService        *mocks.MockModelService
	controllerUUID      string
	controllerModelUUID model.UUID
}

func TestAccessSuite(t *stdtesting.T) {
	tc.Run(t, &accessSuite{})
}

func (s *accessSuite) SetUpTest(c *tc.C) {
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})
	controllerCfg := testing.FakeControllerConfig()

	s.StateSuite.ControllerConfig = controllerCfg
	s.StateSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	s.controllerUUID = modeltesting.GenModelUUID(c).String()
	s.controllerModelUUID = modeltesting.GenModelUUID(c)
}

func (s *accessSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = mocks.NewMockControllerAccessService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	return ctrl
}

func (s *accessSuite) controllerAPI(c *tc.C) *controller.ControllerAPI {

	api, err := controller.NewControllerAPI(
		c.Context(),
		s.State,
		s.StatePool,
		s.authorizer,
		s.resources,
		loggertesting.WrapCheckLog(c),
		nil,
		nil,
		nil,
		s.accessService,
		nil,
		s.modelService,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		s.controllerModelUUID,
		s.controllerUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return api
}

func (s *accessSuite) TestModifyControllerAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userName := usertesting.GenNewName(c, "test-user")

	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.controllerUUID,
			},
		},
		Change:  permission.Grant,
		Subject: userName,
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyControllerAccessRequest{Changes: []params.ModifyControllerAccess{{
		UserTag: names.NewUserTag(userName.Name()).String(),
		Action:  params.GrantControllerAccess,
		Access:  string(permission.SuperuserAccess),
	}}}

	result, err := s.controllerAPI(c).ModifyControllerAccess(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
}

func (s *accessSuite) TestGetControllerAccessPermissions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("test-user")
	userName := user.NameFromTag(userTag)
	differentUser := "different-test-user"

	target := permission.AccessSpec{
		Access: permission.SuperuserAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        s.controllerUUID,
		},
	}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, target.Target).Return(permission.SuperuserAccess, nil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	req := params.Entities{
		Entities: []params.Entity{{Tag: userTag.String()}, {Tag: names.NewUserTag(differentUser).String()}},
	}
	results, err := s.controllerAPI(c).GetControllerAccess(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(*results.Results[0].Result, tc.DeepEquals, params.UserAccess{
		Access:  "superuser",
		UserTag: userTag.String(),
	})
	c.Assert(*results.Results[1].Error, tc.DeepEquals, params.Error{
		Message: "permission denied", Code: "unauthorized access",
	})
}

func (s *accessSuite) TestAllModels(c *tc.C) {
	defer s.setupMocks(c).Finish()
	testAdmin := names.NewUserTag("test-admin")

	models := []model.Model{
		{
			Name:      "controller",
			Qualifier: "admin",
			ModelType: model.IAAS,
		},
		{
			Name:      "no-access",
			Qualifier: "user@remote",
			ModelType: model.IAAS,
		},
		{
			Name:      "owned",
			Qualifier: "admin",
			ModelType: model.IAAS,
		},
		{
			Name:      "user",
			Qualifier: "user@remote",
			ModelType: model.IAAS,
		},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(
		models, nil,
	)

	// api user owner is "test-admin"
	s.accessService.EXPECT().LastModelLogin(gomock.Any(), user.NameFromTag(testAdmin), gomock.Any()).Times(4)

	response, err := s.controllerAPI(c).AllModels(c.Context())
	slices.SortFunc(response.UserModels, func(x params.UserModel, y params.UserModel) int {
		return strings.Compare(x.Name, y.Name)
	})
	c.Assert(err, tc.ErrorIsNil)
	for i, userModel := range response.UserModels {
		c.Assert(userModel.Type, tc.Equals, model.IAAS.String())
		c.Assert(models[i].Name, tc.Equals, userModel.Name)
		c.Assert(models[i].Qualifier.String(), tc.Equals, userModel.Qualifier)
		c.Assert(models[i].ModelType.String(), tc.Equals, userModel.Type)
	}
}

type noopLeadershipReader struct {
	leadership.Reader
}

func (noopLeadershipReader) Leaders() (map[string]string, error) {
	return make(map[string]string), nil
}
