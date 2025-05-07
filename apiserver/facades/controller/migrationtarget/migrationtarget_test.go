// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	_ "github.com/juju/juju/internal/provider/manual"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type Suite struct {
	statetesting.StateSuite
	authorizer *apiservertesting.FakeAuthorizer

	controllerConfigService   *MockControllerConfigService
	domainServices            *MockDomainServices
	domainServicesGetter      *MockDomainServicesGetter
	externalControllerService *MockExternalControllerService
	applicationService        *MockApplicationService
	relationService           *MockRelationService
	statusSerivce             *MockStatusService
	upgradeService            *MockUpgradeService
	modelImporter             *MockModelImporter
	objectStoreGetter         *MockModelObjectStoreGetter
	modelMigrationService     *MockModelMigrationService
	agentService              *MockModelAgentService

	facadeContext facadetest.ModelContext
}

var _ = tc.Suite(&Suite{})

func (s *Suite) SetUpSuite(c *tc.C) {
	c.Skip(`
Skip added by tlm. The reason we are skipping these tests is currently they are
introducing a mock for model import call but then the mock proceeds to actually
call the model import code in internal and do a full end to end tests. These
tests are then running off of domain services mocks.

Eventually we are ending up at a state where we are 6 levels deep in the call
stack writing expect statements. All of these tests need to be refactored
properly into unit tests and not integration tests.

We will get this done as part of dqlite transition.
`)
}

func (s *Suite) SetUpTest(c *tc.C) {
	// Set up InitialConfig with a dummy provider configuration. This
	// is required to allow model import test to work.
	s.InitialConfig = jujutesting.CustomModelConfig(c, jujutesting.FakeConfig())

	// The call to StateSuite's SetUpTest uses s.InitialConfig so
	// it has to happen here.
	s.StateSuite.SetUpTest(c)
}

func (s *Suite) TestFacadeRegistered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	aFactory, err := apiserver.AllFacades().GetFactory("MigrationTarget", 3)
	c.Assert(err, jc.ErrorIsNil)

	api, err := aFactory(context.Background(), &facadetest.MultiModelContext{
		ModelContext: facadetest.ModelContext{
			State_: s.State,
			Auth_:  s.authorizer,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, tc.FitsTypeOf, new(migrationtarget.API))
}

func (s *Suite) importModel(c *tc.C, api *migrationtarget.API) names.ModelTag {
	uuid, bytes := s.makeExportedModel(c)
	err := api.Import(context.Background(), params.SerializedModel{Bytes: bytes})
	c.Assert(err, jc.ErrorIsNil)
	return names.NewModelTag(uuid)
}

func (s *Suite) TestCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	r, err := api.CACert(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(r.Result), tc.Equals, jujutesting.CACert)
}

func (s *Suite) TestPrechecks(c *tc.C) {
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

func (s *Suite) TestPrechecksIsUpgrading(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, `upgrade in progress`)
}

func (s *Suite) TestPrechecksFail(c *tc.C) {
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
	c.Assert(err, tc.NotNil)
}

func (s *Suite) TestPrechecksFacadeVersionsFail(c *tc.C) {
	controllerVersion := s.controllerVersion(c)

	api := s.mustNewAPIWithFacadeVersions(c, facades.FacadeVersions{
		"MigrationTarget": []int{1},
	})
	args := params.MigrationModelInfo{
		AgentVersion:           controllerVersion,
		ControllerAgentVersion: controllerVersion,
	}
	err := api.Prechecks(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, `
Source controller does not support required facades for performing migration.
Upgrade the controller to a newer version of .* or migrate to a controller
with an earlier version of the target controller and try again.

`[1:])
}

func (s *Suite) TestPrechecksFacadeVersionsWithPatchFail(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, `
Source controller does not support required facades for performing migration.
Upgrade the controller to a newer version of .* or migrate to a controller
with an earlier version of the target controller and try again.

`[1:])
}

func (s *Suite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportModel(c)

	api := s.mustNewAPI(c, c.MkDir())
	tag := s.importModel(c, api)
	// Check the model was imported.
	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	c.Assert(model.Name(), tc.Equals, "some-model")
	mode, err := model.State().MigrationMode()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mode, tc.Equals, state.MigrationModeImporting)
}

func (s *Suite) TestAbort(c *tc.C) {
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

func (s *Suite) TestAbortNotATag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	err := api.Abort(context.Background(), params.ModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, tc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestAbortMissingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	newUUID := uuid.MustNewUUID().String()
	err := api.Abort(context.Background(), params.ModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, tc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestAbortNotImportingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	err = api.Abort(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, tc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestActivate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportModel(c)

	sourceModel := "deadbeef-0bad-400d-8000-4b1d0d06f666"
	_, err := commoncrossmodel.GetBackend(s.State).AddRemoteApplication(commoncrossmodel.AddRemoteApplicationParams{
		Name: "foo", SourceModel: names.NewModelTag(sourceModel),
	})
	c.Assert(err, jc.ErrorIsNil)
	api := s.mustNewAPI(c, c.MkDir())
	tag := s.importModel(c, api)

	expectedCI := crossmodel.ControllerInfo{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		Alias:          "mycontroller",
		Addrs:          []string{"10.6.6.6:17070"},
		CACert:         jujutesting.CACert,
		ModelUUIDs:     []string{sourceModel},
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

	mode, err := s.State.MigrationMode()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mode, tc.Equals, state.MigrationModeNone)

	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	app, err := commoncrossmodel.GetBackend(model.State()).RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.SourceController(), tc.Equals, jujutesting.ControllerTag.Id())
}

func (s *Suite) TestActivateNotATag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	err := api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, tc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestActivateMissingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.mustNewAPI(c, c.MkDir())
	newUUID := uuid.MustNewUUID().String()
	err := api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, tc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestActivateNotImportingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	err = api.Activate(context.Background(), params.ActivateModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, tc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestLatestLogTime(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	logDir := c.MkDir()
	t := time.Date(2024, 02, 18, 06, 23, 24, 0, time.UTC)
	logFile := filepath.Join(logDir, "logsink.log")
	err = os.MkdirAll(filepath.Dir(logFile), 0755)
	c.Assert(err, jc.ErrorIsNil)
	// {"timestamp":"2024-02-20T06:01:19.101184262Z","model-uuid":"05756e0f-e5b8-47d3-8093-bf7d53d92589","entity":"machine-0","level":2,"module":"juju.worker.dependency","location":"engine.go:598","message":"\"charmhub-http-client\" manifold worker started at 2024-02-20 06:01:19.10118362 +0000 UTC","labels":null}
	err = os.WriteFile(logFile, []byte("machine-0 2024-02-18 05:00:00 INFO juju.worker worker.go:200 test first\nmachine-0 2024-02-18 06:23:24 INFO juju.worker worker.go:518 test\n bad line"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, logDir)
	latest, err := api.LatestLogTime(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, tc.Equals, t)
}

func (s *Suite) TestLatestLogTimeNeverSet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c, c.MkDir())
	latest, err := api.LatestLogTime(context.Background(), params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, tc.Equals, time.Time{})
}

func (s *Suite) TestAdoptIAASResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	api, err := s.newAPI(facades.FacadeVersions{}, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(context.Background(), params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: semversion.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestAdoptCAASResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()

	api, err := s.newAPI(facades.FacadeVersions{}, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(context.Background(), params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: semversion.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- CheckMachines where machines have instance IDs.
- CheckMachines where some are container-in-machine.
- CheckMachines where there are manually provisioned machines.
- CheckMachines on a manual cloud.`,
	)
}
func (s *Suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujutesting.FakeControllerConfig(), nil).AnyTimes()

	s.domainServices = NewMockDomainServices(ctrl)
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)

	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.statusSerivce = NewMockStatusService(ctrl)
	s.upgradeService = NewMockUpgradeService(ctrl)

	s.objectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.modelImporter = NewMockModelImporter(ctrl)
	s.modelMigrationService = NewMockModelMigrationService(ctrl)

	s.agentService = NewMockModelAgentService(ctrl)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
	s.facadeContext = facadetest.ModelContext{
		State_:         s.State,
		StatePool_:     s.StatePool,
		Auth_:          s.authorizer,
		ModelImporter_: s.modelImporter,
	}

	return ctrl
}

func (s *Suite) migrationServiceGetter(context.Context, model.UUID) (migrationtarget.ModelMigrationService, error) {
	return s.modelMigrationService, nil
}

func (s *Suite) agentServiceGetter(context.Context, model.UUID) (migrationtarget.ModelAgentService, error) {
	return s.agentService, nil
}

func (s *Suite) newAPI(versions facades.FacadeVersions, logDir string) (*migrationtarget.API, error) {
	return migrationtarget.NewAPI(
		&s.facadeContext,
		s.authorizer,
		s.controllerConfigService,
		s.externalControllerService,
		s.applicationService,
		s.relationService,
		s.statusSerivce,
		s.upgradeService,
		s.agentServiceGetter,
		s.migrationServiceGetter,
		versions,
		logDir,
	)
}

func (s *Suite) mustNewAPI(c *tc.C, logDir string) *migrationtarget.API {
	api, err := s.newAPI(facades.FacadeVersions{}, logDir)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) newAPIWithFacadeVersions(versions facades.FacadeVersions, logDir string) (*migrationtarget.API, error) {
	api, err := s.newAPI(versions, logDir)
	return api, err
}

func (s *Suite) mustNewAPIWithFacadeVersions(c *tc.C, versions facades.FacadeVersions) *migrationtarget.API {
	api, err := s.newAPIWithFacadeVersions(versions, c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) makeExportedModel(c *tc.C) (string, []byte) {
	model, err := s.State.Export(jujujujutesting.NewObjectStore(c, s.State.ModelUUID()))
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

func (s *Suite) controllerVersion(*tc.C) semversion.Number {
	return semversion.Number{}
}

func (s *Suite) expectImportModel(c *tc.C) {
	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), gomock.Any()).Return(s.domainServices, nil)
	s.modelImporter.EXPECT().ImportModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, bytes []byte) (*state.Model, *state.State, error) {
		scope := func(model.UUID) modelmigration.Scope { return modelmigration.NewScope(nil, nil, nil) }
		controller := state.NewController(s.StatePool)
		return migration.NewModelImporter(
			controller,
			scope,
			s.controllerConfigService,
			s.domainServicesGetter,
			corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
				return provider.CommonStorageProviders()
			}),
			s.objectStoreGetter,
			loggertesting.WrapCheckLog(c),
			clock.WallClock,
		).ImportModel(ctx, bytes)
	})
}
