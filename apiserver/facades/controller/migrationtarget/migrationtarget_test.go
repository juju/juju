// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/service"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/migration"
	_ "github.com/juju/juju/internal/provider/manual"
	"github.com/juju/juju/internal/uuid"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type Suite struct {
	statetesting.StateSuite
	authorizer *apiservertesting.FakeAuthorizer

	controllerConfigService   *MockControllerConfigService
	serviceFactory            *MockServiceFactory
	serviceFactoryGetter      *MockServiceFactoryGetter
	externalControllerService *MockExternalControllerService
	upgradeService            *MockUpgradeService
	cloudService              *commonmocks.MockCloudService
	credentialService         *credentialcommon.MockCredentialService
	credentialValidator       *MockCredentialValidator
	modelImporter             *MockModelImporter

	facadeContext facadetest.ModelContext
	callContext   envcontext.ProviderCallContext
	leaders       map[string]string
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpSuite(c *gc.C) {
	c.Skip(`
Skip added by tlm. The reason we are skipping these tests is currently they are
introducing a mock for model import call but then the mock proceeds to actually
call the model import code in internal and do a full end to end tests. These
tests are then running off of service factory mocks.

Eventually we are ending up at a state where we are 6 levels deep in the call
stack writing expect statements. All of these tests need to be refactored
properly into unit tests and not integration tests.

We will get this done as part of dqlite transition.
`)
}

func (s *Suite) SetUpTest(c *gc.C) {
	// Set up InitialConfig with a dummy provider configuration. This
	// is required to allow model import test to work.
	s.InitialConfig = jujutesting.CustomModelConfig(c, jujutesting.FakeConfig())

	// The call to StateSuite's SetUpTest uses s.InitialConfig so
	// it has to happen here.
	s.StateSuite.SetUpTest(c)
}

func (s *Suite) TestFacadeRegistered(c *gc.C) {
	defer s.setupMocks(c).Finish()

	aFactory, err := apiserver.AllFacades().GetFactory("MigrationTarget", 3)
	c.Assert(err, jc.ErrorIsNil)

	api, err := aFactory(context.Background(), &facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_:          s.State,
			Auth_:           s.authorizer,
			ServiceFactory_: servicefactorytesting.NewTestingServiceFactory(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.FitsTypeOf, new(migrationtarget.API))
}

func (s *Suite) TestFacadeRegisteredV2(c *gc.C) {
	defer s.setupMocks(c).Finish()

	aFactory, err := apiserver.AllFacades().GetFactory("MigrationTarget", 2)
	c.Assert(err, jc.ErrorIsNil)

	api, err := aFactory(context.Background(), &facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_:          s.State,
			Auth_:           s.authorizer,
			ServiceFactory_: servicefactorytesting.NewTestingServiceFactory(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.FitsTypeOf, new(migrationtarget.APIV2))
}

func (s *Suite) importModel(c *gc.C, api *migrationtarget.API) names.ModelTag {
	uuid, bytes := s.makeExportedModel(c)
	err := api.Import(context.Background(), params.SerializedModel{Bytes: bytes})
	c.Assert(err, jc.ErrorIsNil)
	return names.NewModelTag(uuid)
}

func (s *Suite) TestCACert(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	r, err := api.CACert(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(r.Result), gc.Equals, jujutesting.CACert)
}

func (s *Suite) TestPrechecks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)

	api := s.mustNewAPI(c, c.MkDir())
	args := params.MigrationModelInfo{
		UUID:                   "uuid",
		Name:                   "some-model",
		OwnerTag:               names.NewUserTag("someone").String(),
		AgentVersion:           s.controllerVersion(c),
		ControllerAgentVersion: s.controllerVersion(c),
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestPrechecksIsUpgrading(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)

	api := s.mustNewAPI(c, c.MkDir())
	args := params.MigrationModelInfo{
		UUID:                   "uuid",
		Name:                   "some-model",
		OwnerTag:               names.NewUserTag("someone").String(),
		AgentVersion:           s.controllerVersion(c),
		ControllerAgentVersion: s.controllerVersion(c),
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `upgrade in progress`)
}

func (s *Suite) TestPrechecksFail(c *gc.C) {
	defer s.setupMocks(c).Finish()

	controllerVersion := s.controllerVersion(c)

	// Set the model version ahead of the controller.
	modelVersion := controllerVersion
	modelVersion.Minor++

	api := s.mustNewAPI(c, c.MkDir())
	args := params.MigrationModelInfo{
		AgentVersion: modelVersion,
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, gc.NotNil)
}

func (s *Suite) TestPrechecksFacadeVersionsFail(c *gc.C) {
	controllerVersion := s.controllerVersion(c)

	api := s.mustNewAPIWithFacadeVersions(c, facades.FacadeVersions{
		"MigrationTarget": []int{1},
	})
	args := params.MigrationModelInfo{
		AgentVersion:           controllerVersion,
		ControllerAgentVersion: controllerVersion,
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `
Source controller does not support required facades for performing migration.
Upgrade the controller to a newer version of .* or migrate to a controller
with an earlier version of the target controller and try again.

`[1:])
}

func (s *Suite) TestPrechecksFacadeVersionsWithPatchFail(c *gc.C) {
	controllerVersion := s.controllerVersion(c)
	controllerVersion.Patch++

	api := s.mustNewAPIWithFacadeVersions(c, facades.FacadeVersions{
		"MigrationTarget": []int{1},
	})
	args := params.MigrationModelInfo{
		AgentVersion:           controllerVersion,
		ControllerAgentVersion: controllerVersion,
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `
Source controller does not support required facades for performing migration.
Upgrade the controller to a newer version of .* or migrate to a controller
with an earlier version of the target controller and try again.

`[1:])
}

func (s *Suite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportModel(c)

	api := s.mustNewAPI(c, c.MkDir())
	tag := s.importModel(c, api)
	// Check the model was imported.
	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	c.Assert(model.Name(), gc.Equals, "some-model")
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeImporting)
}

func (s *Suite) TestAbort(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportModel(c)

	api := s.mustNewAPI(c, c.MkDir())
	tag := s.importModel(c, api)

	err := api.Abort(context.Background(), params.ModelArgs{ModelTag: tag.String()})
	c.Assert(err, jc.ErrorIsNil)

	// The model should no longer exist.
	exists, err := s.State.ModelExists(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsFalse)
}

func (s *Suite) TestAbortNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	err := api.Abort(context.Background(), params.ModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, gc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestAbortMissingModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	newUUID := uuid.MustNewUUID().String()
	err := api.Abort(context.Background(), params.ModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, gc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestAbortNotImportingModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	err = api.Abort(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, gc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestActivate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportModel(c)

	sourceModel := "deadbeef-0bad-400d-8000-4b1d0d06f666"
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", SourceModel: names.NewModelTag(sourceModel),
	})
	c.Assert(err, jc.ErrorIsNil)
	api := s.mustNewAPI(c, c.MkDir())
	tag := s.importModel(c, api)

	expectedCI := crossmodel.ControllerInfo{
		ControllerTag: jujutesting.ControllerTag,
		Alias:         "mycontroller",
		Addrs:         []string{"10.6.6.6:17070"},
		CACert:        jujutesting.CACert,
		ModelUUIDs:    []string{sourceModel},
	}
	s.externalControllerService.EXPECT().UpdateExternalController(
		gomock.Any(),
		expectedCI,
	).Times(1)
	s.externalControllerService.EXPECT().ControllerForModel(
		gomock.Any(),
		sourceModel,
	).Times(1).Return(&expectedCI, nil)

	err = api.Activate(context.Background(), params.ActivateModelArgs{
		ModelTag:        tag.String(),
		ControllerTag:   jujutesting.ControllerTag.String(),
		ControllerAlias: "mycontroller",
		SourceAPIAddrs:  []string{"10.6.6.6:17070"},
		SourceCACert:    jujutesting.CACert,
		CrossModelUUIDs: []string{sourceModel},
	})
	c.Assert(err, jc.ErrorIsNil)

	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)

	app, err := model.State().RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.SourceController(), gc.Equals, jujutesting.ControllerTag.Id())
}

func (s *Suite) TestActivateNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	err := api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, gc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestActivateMissingModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	newUUID := uuid.MustNewUUID().String()
	err := api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, gc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestActivateNotImportingModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	err = api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, gc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestLatestLogTime(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	logDir := c.MkDir()
	t := time.Date(2024, 02, 18, 06, 23, 24, 0, time.UTC)
	logFile := corelogger.ModelLogFile(logDir, model.UUID(), corelogger.ModelFilePrefix(model.Owner().Id(), model.Name()))
	err = os.MkdirAll(filepath.Dir(logFile), 0755)
	c.Assert(err, jc.ErrorIsNil)
	// {"timestamp":"2024-02-20T06:01:19.101184262Z","model-uuid":"05756e0f-e5b8-47d3-8093-bf7d53d92589","entity":"machine-0","level":2,"module":"juju.worker.dependency","location":"engine.go:598","message":"\"charmhub-http-client\" manifold worker started at 2024-02-20 06:01:19.10118362 +0000 UTC","labels":null}
	err = os.WriteFile(logFile, []byte("machine-0 2024-02-18 05:00:00 INFO juju.worker worker.go:200 test first\nmachine-0 2024-02-18 06:23:24 INFO juju.worker worker.go:518 test\n bad line"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, logDir)
	latest, err := api.LatestLogTime(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, t)
}

func (s *Suite) TestLatestLogTimeNeverSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	latest, err := api.LatestLogTime(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, time.Time{})
}

func (s *Suite) TestAdoptIAASResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	env := mockEnv{Stub: &testing.Stub{}}
	api, err := s.newAPI(func(model stateenvirons.Model, _ stateenvirons.CloudService, _ stateenvirons.CredentialService) (environs.Environ, error) {
		c.Assert(model.ModelTag().Id(), gc.Equals, st.ModelUUID())
		return &env, nil
	}, func(model stateenvirons.Model, _ stateenvirons.CloudService, _ stateenvirons.CredentialService) (caas.Broker, error) {
		return nil, errors.New("should not be called")
	}, facades.FacadeVersions{}, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(context.Background(), params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: version.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Stub.Calls(), gc.HasLen, 1)
	aCall := env.Stub.Calls()[0]
	c.Assert(aCall.FuncName, gc.Equals, "AdoptResources")
	c.Assert(aCall.Args[1], gc.Equals, st.ControllerUUID())
	c.Assert(aCall.Args[2], gc.Equals, version.MustParse("3.2.1"))
}

func (s *Suite) TestAdoptCAASResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()

	broker := mockBroker{Stub: &testing.Stub{}}
	api, err := s.newAPI(func(model stateenvirons.Model, _ stateenvirons.CloudService, _ stateenvirons.CredentialService) (environs.Environ, error) {
		return nil, errors.New("should not be called")
	}, func(model stateenvirons.Model, _ stateenvirons.CloudService, _ stateenvirons.CredentialService) (caas.Broker, error) {
		c.Assert(model.ModelTag().Id(), gc.Equals, st.ModelUUID())
		return &broker, nil
	}, facades.FacadeVersions{}, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(context.Background(), params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: version.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(broker.Stub.Calls(), gc.HasLen, 1)
	aCall := broker.Stub.Calls()[0]
	c.Assert(aCall.FuncName, gc.Equals, "AdoptResources")
	c.Assert(aCall.Args[1], gc.Equals, st.ControllerUUID())
	c.Assert(aCall.Args[2], gc.Equals, version.MustParse("3.2.1"))
}

func (s *Suite) TestCheckMachinesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "eriatarka",
	})
	m := fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "volta",
	})
	c.Assert(m.Id(), gc.Equals, "1")

	mockEnv := mockEnv{
		Stub: &testing.Stub{},
		instances: []*mockInstance{
			{id: "volta"},
			{id: "eriatarka"},
		},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		context.Background(),
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) TestCheckMachinesHandlesContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	m := fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "birds",
	})
	fact.MakeMachineNested(c, m.Id(), nil)

	mockEnv := mockEnv{
		Stub:      &testing.Stub{},
		instances: []*mockInstance{{id: "birds"}},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		context.Background(),
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) TestCheckMachinesIgnoresManualMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "birds",
	})
	fact.MakeMachine(c, &factory.MachineParams{
		Nonce: "manual:flibbertigibbert",
	})

	mockEnv := mockEnv{
		Stub:      &testing.Stub{},
		instances: []*mockInstance{{id: "birds"}},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		context.Background(),
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) TestCheckMachinesManualCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.cloudService.EXPECT().Get(gomock.Any(), "manual").Return(&cloud.Cloud{
		Name:      "manual",
		Type:      "manual",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		Endpoint:  "10.0.0.1",
	}, nil)

	owner := s.Factory.MakeUser(c, nil)

	cred := cloud.NewCredential(cloud.EmptyAuthType, nil)
	tag := names.NewCloudCredentialTag(
		fmt.Sprintf("manual/%s/dummy-credential", owner.Name()))
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.IdFromTag(tag)).Return(cred, nil)
	s.credentialValidator.EXPECT().Validate(gomock.Any(), gomock.Any(), credential.IdFromTag(tag), &cred, false)

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		CloudName:       "manual",
		CloudCredential: tag,
		Owner:           owner.UserTag(),
	})
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	fact.MakeMachine(c, &factory.MachineParams{
		Nonce: "manual:birds",
	})
	fact.MakeMachine(c, &factory.MachineParams{
		Nonce: "manual:flibbertigibbert",
	})

	mockEnv := mockEnv{
		Stub:      &testing.Stub{},
		instances: []*mockInstance{{id: "birds"}, {id: "flibbertigibbert"}},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		context.Background(),
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *Suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujutesting.FakeControllerConfig(), nil).AnyTimes()

	s.serviceFactory = NewMockServiceFactory(ctrl)
	s.serviceFactoryGetter = NewMockServiceFactoryGetter(ctrl)

	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.cloudService.EXPECT().Get(gomock.Any(), "dummy").Return(&cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		Endpoint:  "10.0.0.1",
	}, nil).AnyTimes()
	s.credentialService = credentialcommon.NewMockCredentialService(ctrl)
	s.credentialValidator = NewMockCredentialValidator(ctrl)

	s.modelImporter = NewMockModelImporter(ctrl)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
	s.callContext = envcontext.WithoutCredentialInvalidator(context.Background())
	s.facadeContext = facadetest.ModelContext{
		State_:         s.State,
		StatePool_:     s.StatePool,
		Auth_:          s.authorizer,
		ModelImporter_: s.modelImporter,
	}

	s.leaders = map[string]string{}

	return ctrl
}

func (s *Suite) newAPI(environFunc stateenvirons.NewEnvironFunc, brokerFunc stateenvirons.NewCAASBrokerFunc, versions facades.FacadeVersions, logDir string) (*migrationtarget.API, error) {
	return migrationtarget.NewAPI(
		&s.facadeContext,
		s.authorizer,
		s.controllerConfigService,
		s.externalControllerService,
		s.upgradeService,
		s.cloudService,
		s.credentialService,
		s.credentialValidator,
		func(ctx context.Context, modelUUID coremodel.UUID) (service.CredentialValidationContext, error) {
			return service.CredentialValidationContext{}, nil
		},
		func() (envcontext.ModelCredentialInvalidatorFunc, error) {
			return func(_ context.Context, reason string) error {
				return nil
			}, nil
		},
		environFunc,
		brokerFunc,
		versions,
		logDir,
	)
}

func (s *Suite) mustNewAPI(c *gc.C, logDir string) *migrationtarget.API {
	api, err := s.newAPI(nil, nil, facades.FacadeVersions{}, logDir)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) newAPIWithFacadeVersions(environFunc stateenvirons.NewEnvironFunc, brokerFunc stateenvirons.NewCAASBrokerFunc, versions facades.FacadeVersions, logDir string) (*migrationtarget.API, error) {
	api, err := s.newAPI(environFunc, brokerFunc, versions, logDir)
	return api, err
}

func (s *Suite) mustNewAPIWithFacadeVersions(c *gc.C, versions facades.FacadeVersions) *migrationtarget.API {
	api, err := s.newAPIWithFacadeVersions(nil, nil, versions, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) mustNewAPIWithModel(c *gc.C, env environs.Environ, broker caas.Broker) *migrationtarget.API {
	api, err := s.newAPI(func(stateenvirons.Model, stateenvirons.CloudService, stateenvirons.CredentialService) (environs.Environ, error) {
		return env, nil
	}, func(stateenvirons.Model, stateenvirons.CloudService, stateenvirons.CredentialService) (caas.Broker, error) {
		return broker, nil
	}, facades.FacadeVersions{}, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) makeExportedModel(c *gc.C) (string, []byte) {
	model, err := s.State.Export(s.leaders, jujujujutesting.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	newUUID := uuid.MustNewUUID().String()
	model.UpdateConfig(map[string]interface{}{
		"name": "some-model",
		"uuid": newUUID,
	})

	bytes, err := description.Serialize(model)
	c.Assert(err, jc.ErrorIsNil)
	return newUUID, bytes
}

func (s *Suite) controllerVersion(c *gc.C) version.Number {
	cfg, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	vers, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	return vers
}

func (s *Suite) expectImportModel(c *gc.C) {
	s.serviceFactoryGetter.EXPECT().FactoryForModel(gomock.Any()).Return(s.serviceFactory)
	s.modelImporter.EXPECT().ImportModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, bytes []byte) (*state.Model, *state.State, error) {
		scope := func(string) modelmigration.Scope { return modelmigration.NewScope(nil, nil) }
		controller := state.NewController(s.StatePool)
		return migration.NewModelImporter(controller, scope, s.controllerConfigService, s.serviceFactoryGetter, cloudSchemaSource).ImportModel(ctx, bytes)
	})
}

func cloudSchemaSource(stateenvirons.CloudService) config.ConfigSchemaSourceGetter {
	return state.NoopConfigSchemaSource
}

type mockEnv struct {
	environs.Environ
	*testing.Stub

	instances []*mockInstance
}

func (e *mockEnv) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, sourceVersion version.Number) error {
	e.MethodCall(e, "AdoptResources", ctx, controllerUUID, sourceVersion)
	return e.NextErr()
}

func (e *mockEnv) AllInstances(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
	e.MethodCall(e, "AllInstances", ctx)
	results := make([]instances.Instance, len(e.instances))
	for i, anInstance := range e.instances {
		results[i] = anInstance
	}
	return results, e.NextErr()
}

type mockBroker struct {
	caas.Broker
	*testing.Stub
}

func (e *mockBroker) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, sourceVersion version.Number) error {
	e.MethodCall(e, "AdoptResources", ctx, controllerUUID, sourceVersion)
	return e.NextErr()
}

type mockInstance struct {
	instances.Instance
	id string
}

func (i *mockInstance) Id() instance.Id {
	return instance.Id(i.id)
}
