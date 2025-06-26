// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type Suite struct {
	coretesting.BaseSuite

	backend         *mocks.MockBackend
	modelExporter   *mocks.MockModelExporter
	precheckBackend *mocks.MockPrecheckBackend
	store           *mocks.MockObjectStore

	agentService            *mocks.MockModelAgentService
	applicationService      *mocks.MockApplicationService
	relationService         *mocks.MockRelationService
	statusService           *mocks.MockStatusService
	controllerConfigService *mocks.MockControllerConfigService
	controllerNodeService   *mocks.MockControllerNodeService
	credentialService       *mocks.MockCredentialService
	modelInfoService        *mocks.MockModelInfoService
	modelService            *mocks.MockModelService
	upgradeService          *mocks.MockUpgradeService

	controllerModelUUID model.UUID
	controllerUUID      string
	modelUUID           string
	model               description.Model
	resources           *common.Resources
	authorizer          apiservertesting.FakeAuthorizer
	cloudSpec           environscloudspec.CloudSpec
}

func TestSuite(t *testing.T) {
	tc.Run(t, &Suite{})
}

func (s *Suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.controllerModelUUID = modeltesting.GenModelUUID(c)
	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = uuid.MustNewUUID().String()

	s.model = description.NewModel(description.ModelArgs{
		Type:               "iaas",
		Config:             map[string]interface{}{"uuid": s.modelUUID},
		Owner:              "admin",
		LatestToolsVersion: jujuversion.Current.String(),
	})

	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{Controller: true}
	s.cloudSpec = environscloudspec.CloudSpec{Type: "lxd"}

}

func (s *Suite) TestNotController(c *tc.C) {
	s.authorizer.Controller = false

	api, err := s.makeAPI()
	c.Assert(api, tc.IsNil)
	c.Assert(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestWatch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Watcher with an initial event in the pipe.
	w := mocks.NewMockNotifyWatcher(ctrl)
	w.EXPECT().Kill().AnyTimes()
	w.EXPECT().Wait().Return(nil).AnyTimes()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w.EXPECT().Changes().Return(ch).Times(2)

	s.backend.EXPECT().WatchForMigration().Return(w)

	result := s.mustMakeAPI(c).Watch(c.Context())
	c.Assert(result.Error, tc.IsNil)

	resource := s.resources.Get(result.NotifyWatcherId)
	watcher, _ := resource.(state.NotifyWatcher)
	c.Assert(watcher, tc.NotNil)

	select {
	case <-watcher.Changes():
		c.Fatalf("initial event not consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *Suite) TestMigrationStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	password := "secret"
	token := "token"

	mig := mocks.NewMockModelMigration(ctrl)

	mac, err := macaroon.New([]byte(password), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)

	targetInfo := coremigration.TargetInfo{
		ControllerTag: names.NewControllerTag(s.controllerUUID),
		Addrs:         []string{"1.1.1.1:1", "2.2.2.2:2"},
		CACert:        "trust me",
		AuthTag:       names.NewUserTag("admin"),
		Password:      password,
		Macaroons:     []macaroon.Slice{{mac}},
		Token:         token,
	}

	exp := mig.EXPECT()
	exp.TargetInfo().Return(&targetInfo, nil)
	exp.Phase().Return(coremigration.IMPORT, nil)
	exp.ModelUUID().Return(s.modelUUID)
	exp.Id().Return("ID")
	now := time.Now()
	exp.PhaseChangedTime().Return(now)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	api := s.mustMakeAPI(c)
	status, err := api.MigrationStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(status, tc.DeepEquals, params.MasterMigrationStatus{
		Spec: params.MigrationSpec{
			ModelTag: names.NewModelTag(s.modelUUID).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(s.controllerUUID).String(),
				Addrs:         []string{"1.1.1.1:1", "2.2.2.2:2"},
				CACert:        "trust me",
				AuthTag:       names.NewUserTag("admin").String(),
				Password:      password,
				Macaroons:     `[[{"l":"location","i":"id","s64":"qYAr8nQmJzPWKDppxigFtWaNv0dbzX7cJaligz98LLo"}]]`,
				Token:         token,
			},
		},
		MigrationId:      "ID",
		Phase:            "IMPORT",
		PhaseChangedTime: now,
	})
}

func (s *Suite) TestModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		UUID:            "model-uuid",
		Name:            "model-name",
		Qualifier:       "production",
		CredentialOwner: usertesting.GenNewName(c, "owner"),
		AgentVersion:    semversion.MustParse("1.2.3"),
	}, nil)

	modelDescription := description.NewModel(description.ModelArgs{})
	s.modelExporter.EXPECT().ExportModel(gomock.Any(), gomock.Any()).Return(modelDescription, nil)

	mod, err := s.mustMakeAPI(c).ModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(mod.UUID, tc.Equals, "model-uuid")
	c.Check(mod.Name, tc.Equals, "model-name")
	c.Check(mod.Qualifier, tc.Equals, "production")
	c.Check(mod.AgentVersion, tc.Equals, semversion.MustParse("1.2.3"))

	bytes, err := description.Serialize(modelDescription)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.ModelDescription, tc.DeepEquals, bytes)
}

func (s *Suite) TestSourceControllerInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := controller.Config{
		controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
		controller.ControllerName:    "mycontroller",
		controller.CACertKey:         "cacert",
	}

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)
	apiAddr := []string{"10.0.0.1:666"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForClients(gomock.Any()).Return(apiAddr, nil)

	info, err := s.mustMakeAPI(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info, tc.DeepEquals, params.MigrationSourceInfo{
		ControllerTag:   coretesting.ControllerTag.String(),
		ControllerAlias: "mycontroller",
		Addrs:           []string{"10.0.0.1:666"},
		CACert:          "cacert",
	})
}

func (s *Suite) TestSetPhase(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetPhase(coremigration.ABORT).Return(nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetPhase(c.Context(), params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, tc.ErrorIsNil)

}

func (s *Suite) TestSetPhaseNoMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().LatestMigration().Return(nil, errors.New("boom"))

	err := s.mustMakeAPI(c).SetPhase(c.Context(), params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, tc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetPhaseBadPhase(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetPhase(c.Context(), params.SetMigrationPhaseArgs{Phase: "wat"})
	c.Assert(err, tc.ErrorMatches, `invalid phase: "wat"`)
}

func (s *Suite) TestSetPhaseError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetPhase(coremigration.ABORT).Return(errors.New("blam"))

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetPhase(c.Context(), params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, tc.ErrorMatches, "failed to set phase: blam")
}

func (s *Suite) TestSetStatusMessage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetStatusMessage("foo").Return(nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetStatusMessage(c.Context(), params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *Suite) TestSetStatusMessageNoMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().LatestMigration().Return(nil, errors.New("boom"))

	err := s.mustMakeAPI(c).SetStatusMessage(c.Context(), params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, tc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetStatusMessageError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetStatusMessage("foo").Return(errors.New("blam"))

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetStatusMessage(c.Context(), params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, tc.ErrorMatches, "failed to set status message: blam")
}

func (s *Suite) TestPrechecksModelError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{}, errors.New("boom"))

	err := s.mustMakeAPI(c).Prechecks(c.Context(), params.PrechecksArgs{TargetControllerVersion: semversion.MustParse("2.9.32")})
	c.Assert(err, tc.ErrorMatches, "retrieving model info: boom")
}

func (s *Suite) TestProcessRelations(c *tc.C) {
	api := s.mustMakeAPI(c)
	err := api.ProcessRelations(c.Context(), params.ProcessRelations{ControllerAlias: "foo"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *Suite) TestExportIAAS(c *tc.C) {
	s.assertExport(c, "iaas")
}

func (s *Suite) TestExportCAAS(c *tc.C) {
	s.model = description.NewModel(description.ModelArgs{
		Type:               "caas",
		Config:             map[string]interface{}{"uuid": s.modelUUID},
		Owner:              "admin",
		LatestToolsVersion: jujuversion.Current.String(),
	})
	s.assertExport(c, "caas")
}

func (s *Suite) assertExport(c *tc.C, modelType string) {
	defer s.setupMocks(c).Finish()

	app := s.model.AddApplication(description.ApplicationArgs{
		Name:     "foo",
		CharmURL: "ch:foo-0",
	})

	const tools0 = "2.0.0-ubuntu-amd64"
	const tools1 = "2.0.1-ubuntu-amd64"
	const tools2 = "2.0.2-ubuntu-amd64"
	m := s.model.AddMachine(description.MachineArgs{Id: "9"})
	m.SetTools(description.AgentToolsArgs{
		Version: tools1,
		SHA256:  "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e",
	})
	c1 := m.AddContainer(description.MachineArgs{Id: "9/lxd/0"})
	c1.SetTools(description.AgentToolsArgs{
		Version: tools2,
		SHA256:  "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3daaa",
	})
	c2 := m.AddContainer(description.MachineArgs{Id: "9/lxd/1"})
	c2.SetTools(description.AgentToolsArgs{
		Version: tools1,
		SHA256:  "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e",
	})

	res := app.AddResource(description.ResourceArgs{Name: "bin"})
	appRev := res.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:    2,
		Type:        "file",
		Origin:      "upload",
		SHA384:      "abcd",
		Size:        123,
		Timestamp:   time.Now(),
		RetrievedBy: "bob",
	})

	unit := app.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})
	unit.SetTools(description.AgentToolsArgs{
		Version: tools0,
		SHA256:  "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3dbbb",
	})

	s.modelExporter.EXPECT().ExportModel(gomock.Any(), s.store).Return(s.model, nil)

	serialized, err := s.mustMakeAPI(c).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// We don't want to tie this test the serialisation output (that's
	// tested elsewhere). Just check that at least one thing we expect
	// is in the serialised output.
	c.Check(string(serialized.Bytes), tc.Contains, jujuversion.Current.String())

	c.Check(serialized.Charms, tc.DeepEquals, []string{"ch:foo-0"})
	if modelType == "caas" {
		c.Check(serialized.Tools, tc.HasLen, 0)
	} else {
		c.Check(serialized.Tools, tc.SameContents, []params.SerializedModelTools{
			{Version: tools0, URI: "/tools/" + tools0, SHA256: "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3dbbb"},
			{Version: tools1, URI: "/tools/" + tools1, SHA256: "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"},
			{Version: tools2, URI: "/tools/" + tools2, SHA256: "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3daaa"},
		})
	}
	c.Check(serialized.Resources, tc.DeepEquals, []params.SerializedModelResource{{
		Application:    "foo",
		Name:           "bin",
		Revision:       appRev.Revision(),
		Type:           appRev.Type(),
		Origin:         appRev.Origin(),
		FingerprintHex: appRev.SHA384(),
		Size:           appRev.Size(),
		Timestamp:      appRev.Timestamp(),
		Username:       appRev.RetrievedBy(),
	}})
}

func (s *Suite) TestReap(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)

	exp := s.backend.EXPECT()
	exp.LatestMigration().Return(mig, nil)

	// Reaping should set the migration phase to DONE - otherwise
	// there's a race between the migrationmaster worker updating the
	// phase and being stopped because the model's gone. This leaves
	// the migration as active in the source controller, which will
	// prevent the model from being migrated back.
	exp.RemoveExportingModelDocs().Return(nil)
	mig.EXPECT().SetPhase(coremigration.DONE).Return(nil)

	err := s.mustMakeAPI(c).Reap(c.Context())
	c.Check(err, tc.ErrorIsNil)

}

func (s *Suite) TestReapError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)
	s.backend.EXPECT().RemoveExportingModelDocs().Return(errors.New("boom"))

	err := s.mustMakeAPI(c).Reap(c.Context())
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *Suite) TestWatchMinionReports(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Watcher with an initial event in the pipe.
	w := mocks.NewMockNotifyWatcher(ctrl)
	w.EXPECT().Kill().AnyTimes()
	w.EXPECT().Wait().Return(nil).AnyTimes()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w.EXPECT().Changes().Return(ch).Times(2)

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().WatchMinionReports().Return(w, nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	result := s.mustMakeAPI(c).WatchMinionReports(c.Context())
	c.Assert(result.Error, tc.IsNil)

	resource := s.resources.Get(result.NotifyWatcherId)
	watcher, _ := resource.(state.NotifyWatcher)
	c.Assert(watcher, tc.NotNil)

	select {
	case <-watcher.Changes():
		c.Fatalf("initial event not consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *Suite) TestMinionReports(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Report 16 unknowns.
	// These are in reverse order in order to test sorting.
	unknown := make([]names.Tag, 0, 16)
	for i := cap(unknown) - 1; i >= 0; i-- {
		unknown = append(unknown, names.NewMachineTag(fmt.Sprintf("%d", i)))
	}
	m50c0 := names.NewMachineTag("50/lxd/0")
	m50c1 := names.NewMachineTag("50/lxd/1")
	m50 := names.NewMachineTag("50")
	m51 := names.NewMachineTag("51")
	m52 := names.NewMachineTag("52")
	u0 := names.NewUnitTag("foo/0")
	u1 := names.NewUnitTag("foo/1")

	mig := mocks.NewMockModelMigration(ctrl)

	exp := mig.EXPECT()
	exp.Id().Return("ID")
	exp.Phase().Return(coremigration.IMPORT, nil)
	exp.MinionReports().Return(&state.MinionReports{
		Succeeded: []names.Tag{m50, m51, u0},
		Failed:    []names.Tag{u1, m52, m50c1, m50c0},
		Unknown:   unknown,
	}, nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	reports, err := s.mustMakeAPI(c).MinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Expect the sample of unknowns to be in order and be limited to
	// the first 10.
	expectedSample := make([]string, 0, 10)
	for i := 0; i < cap(expectedSample); i++ {
		expectedSample = append(expectedSample, names.NewMachineTag(fmt.Sprintf("%d", i)).String())
	}
	c.Assert(reports, tc.DeepEquals, params.MinionReports{
		MigrationId:   "ID",
		Phase:         "IMPORT",
		SuccessCount:  3,
		UnknownCount:  len(unknown),
		UnknownSample: expectedSample,
		Failed: []string{
			// Note sorting.
			m50c0.String(),
			m50c1.String(),
			m52.String(),
			u1.String(),
		},
	})
}

func (s *Suite) TestMinionReportTimeout(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	timeout := "30s"

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.MigrationMinionWaitMax: timeout,
	}, nil)

	res, err := s.mustMakeAPI(c).MinionReportTimeout(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)
	c.Check(res.Result, tc.Equals, timeout)
}

func (s *Suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentService = mocks.NewMockModelAgentService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.relationService = mocks.NewMockRelationService(ctrl)
	s.statusService = mocks.NewMockStatusService(ctrl)
	s.backend = mocks.NewMockBackend(ctrl)
	s.credentialService = mocks.NewMockCredentialService(ctrl)
	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.controllerNodeService = mocks.NewMockControllerNodeService(ctrl)
	s.modelExporter = mocks.NewMockModelExporter(ctrl)
	s.modelInfoService = mocks.NewMockModelInfoService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.precheckBackend = mocks.NewMockPrecheckBackend(ctrl)
	s.store = mocks.NewMockObjectStore(ctrl)
	s.upgradeService = mocks.NewMockUpgradeService(ctrl)

	c.Cleanup(func() {
		s.agentService = nil
		s.applicationService = nil
		s.relationService = nil
		s.statusService = nil
		s.backend = nil
		s.credentialService = nil
		s.controllerConfigService = nil
		s.modelExporter = nil
		s.modelInfoService = nil
		s.modelService = nil
		s.precheckBackend = nil
		s.store = nil
		s.upgradeService = nil
	})
	return ctrl
}

func (s *Suite) mustMakeAPI(c *tc.C) *migrationmaster.API {
	api, err := s.makeAPI()
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *Suite) makeAPI() (*migrationmaster.API, error) {
	return migrationmaster.NewAPI(
		s.backend,
		s.modelExporter,
		s.store,
		s.controllerModelUUID,
		s.precheckBackend,
		nil, // pool
		s.resources,
		s.authorizer,
		stubLeadership{},
		func(context.Context, model.UUID) (migrationmaster.CredentialService, error) {
			return s.credentialService, nil
		},
		func(context.Context, model.UUID) (migrationmaster.UpgradeService, error) {
			return s.upgradeService, nil
		},
		func(context.Context, model.UUID) (migrationmaster.ApplicationService, error) {
			return s.applicationService, nil
		},
		func(context.Context, model.UUID) (migrationmaster.RelationService, error) {
			return s.relationService, nil
		},
		func(context.Context, model.UUID) (migrationmaster.StatusService, error) {
			return s.statusService, nil
		},
		func(context.Context, model.UUID) (migrationmaster.ModelAgentService, error) {
			return s.agentService, nil
		},
		s.controllerConfigService,
		s.controllerNodeService,
		s.modelInfoService,
		s.modelService,
	)
}

type stubLeadership struct{}

func (stubLeadership) Leaders() (map[string]string, error) {
	return map[string]string{}, nil
}
