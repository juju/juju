// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"fmt"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/presence"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type Suite struct {
	coretesting.BaseSuite

	controllerBackend *mocks.MockControllerState
	backend           *mocks.MockBackend
	precheckBackend   *mocks.MockPrecheckBackend

	controllerUUID string
	modelUUID      string
	model          description.Model
	resources      *common.Resources
	authorizer     apiservertesting.FakeAuthorizer
	cloudSpec      environscloudspec.CloudSpec
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.controllerUUID = utils.MustNewUUID().String()
	s.modelUUID = utils.MustNewUUID().String()

	s.model = description.NewModel(description.ModelArgs{
		Type:               "iaas",
		Config:             map[string]interface{}{"uuid": s.modelUUID},
		Owner:              names.NewUserTag("admin"),
		LatestToolsVersion: jujuversion.Current,
	})

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{Controller: true}
	s.cloudSpec = environscloudspec.CloudSpec{Type: "lxd"}
}

func (s *Suite) TestNotController(c *gc.C) {
	s.authorizer.Controller = false

	api, err := s.makeAPI()
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestWatch(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Watcher with an initial event in the pipe.
	w := mocks.NewMockNotifyWatcher(ctrl)
	w.EXPECT().Stop().Return(nil).AnyTimes()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w.EXPECT().Changes().Return(ch).Times(2)

	s.backend.EXPECT().WatchForMigration().Return(w)

	result := s.mustMakeAPI(c).Watch()
	c.Assert(result.Error, gc.IsNil)

	resource := s.resources.Get(result.NotifyWatcherId)
	watcher, _ := resource.(state.NotifyWatcher)
	c.Assert(watcher, gc.NotNil)

	select {
	case <-watcher.Changes():
		c.Fatalf("initial event not consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *Suite) TestMigrationStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	password := "secret"
	token := "token"

	mig := mocks.NewMockModelMigration(ctrl)

	mac, err := macaroon.New([]byte(password), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

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
	status, err := api.MigrationStatus()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status, gc.DeepEquals, params.MasterMigrationStatus{
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

func (s *Suite) TestModelInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelDescription := description.NewModel(description.ModelArgs{})

	exp := s.backend.EXPECT()
	exp.ModelUUID().Return("model-uuid")
	exp.ModelName().Return("model-name", nil)
	exp.ModelOwner().Return(names.NewUserTag("owner"), nil)
	exp.AgentVersion().Return(version.MustParse("1.2.3"), nil)
	exp.Export(gomock.Any()).Return(modelDescription, nil)

	mod, err := s.mustMakeAPI(c).ModelInfo()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mod.UUID, gc.Equals, "model-uuid")
	c.Check(mod.Name, gc.Equals, "model-name")
	c.Check(mod.OwnerTag, gc.Equals, names.NewUserTag("owner").String())
	c.Check(mod.AgentVersion, gc.Equals, version.MustParse("1.2.3"))

	bytes, err := description.Serialize(modelDescription)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.ModelDescription, gc.DeepEquals, bytes)
}

func (s *Suite) TestSourceControllerInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.backend.EXPECT()
	exp.AllLocalRelatedModels().Return([]string{"related-model-uuid"}, nil)
	s.backend.EXPECT().ControllerConfig().Return(controller.Config{
		controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
		controller.ControllerName:    "mycontroller",
		controller.CACertKey:         "cacert",
	}, nil)
	apiAddr := []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{Value: "10.0.0.1"},
		},
		NetPort: 666,
	}}}
	s.controllerBackend.EXPECT().APIHostPortsForClients().Return(apiAddr, nil)

	info, err := s.mustMakeAPI(c).SourceControllerInfo()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info, jc.DeepEquals, params.MigrationSourceInfo{
		LocalRelatedModels: []string{"related-model-uuid"},
		ControllerTag:      coretesting.ControllerTag.String(),
		ControllerAlias:    "mycontroller",
		Addrs:              []string{"10.0.0.1:666"},
		CACert:             "cacert",
	})
}

func (s *Suite) TestSetPhase(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetPhase(coremigration.ABORT).Return(nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, jc.ErrorIsNil)

}

func (s *Suite) TestSetPhaseNoMigration(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().LatestMigration().Return(nil, errors.New("boom"))

	err := s.mustMakeAPI(c).SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetPhaseBadPhase(c *gc.C) {
	err := s.mustMakeAPI(c).SetPhase(params.SetMigrationPhaseArgs{Phase: "wat"})
	c.Assert(err, gc.ErrorMatches, `invalid phase: "wat"`)
}

func (s *Suite) TestSetPhaseError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetPhase(coremigration.ABORT).Return(errors.New("blam"))

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "failed to set phase: blam")
}

func (s *Suite) TestSetStatusMessage(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetStatusMessage("foo").Return(nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestSetStatusMessageNoMigration(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().LatestMigration().Return(nil, errors.New("boom"))

	err := s.mustMakeAPI(c).SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, gc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetStatusMessageError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().SetStatusMessage("foo").Return(errors.New("blam"))

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	err := s.mustMakeAPI(c).SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, gc.ErrorMatches, "failed to set status message: blam")
}

func (s *Suite) TestPrechecksModelError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.precheckBackend.EXPECT().Model().Return(nil, errors.New("boom"))

	err := s.mustMakeAPI(c).Prechecks(params.PrechecksArgs{TargetControllerVersion: version.MustParse("2.9.32")})
	c.Assert(err, gc.ErrorMatches, "retrieving model: boom")
}

func (s *Suite) TestProcessRelations(c *gc.C) {
	api := s.mustMakeAPI(c)
	err := api.ProcessRelations(params.ProcessRelations{ControllerAlias: "foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestExportIAAS(c *gc.C) {
	s.assertExport(c, "iaas")
}

func (s *Suite) TestExportCAAS(c *gc.C) {
	s.model = description.NewModel(description.ModelArgs{
		Type:               "caas",
		Config:             map[string]interface{}{"uuid": s.modelUUID},
		Owner:              names.NewUserTag("admin"),
		LatestToolsVersion: jujuversion.Current,
	})
	s.assertExport(c, "caas")
}

func (s *Suite) assertExport(c *gc.C, modelType string) {
	defer s.setupMocks(c).Finish()

	app := s.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("foo"),
		CharmURL: "ch:foo-0",
	})

	const tools0 = "2.0.0-ubuntu-amd64"
	const tools1 = "2.0.1-ubuntu-amd64"
	m := s.model.AddMachine(description.MachineArgs{Id: names.NewMachineTag("9")})
	m.SetTools(description.AgentToolsArgs{
		Version: version.MustParseBinary(tools1),
	})

	res := app.AddResource(description.ResourceArgs{Name: "bin"})
	appRev := res.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:       2,
		Type:           "file",
		Path:           "bin.tar.gz",
		Description:    "who knows",
		Origin:         "upload",
		FingerprintHex: "abcd",
		Size:           123,
		Timestamp:      time.Now(),
		Username:       "bob",
	})
	csRev := res.SetCharmStoreRevision(description.ResourceRevisionArgs{
		Revision:       3,
		Type:           "file",
		Path:           "fink.tar.gz",
		Description:    "knows who",
		Origin:         "store",
		FingerprintHex: "deaf",
		Size:           321,
		Timestamp:      time.Now(),
		Username:       "xena",
	})

	unit := app.AddUnit(description.UnitArgs{
		Tag: names.NewUnitTag("foo/0"),
	})
	unit.SetTools(description.AgentToolsArgs{
		Version: version.MustParseBinary(tools0),
	})
	unitRes := unit.AddResource(description.UnitResourceArgs{
		Name: "bin",
		RevisionArgs: description.ResourceRevisionArgs{
			Revision:       1,
			Type:           "file",
			Path:           "bin.tar.gz",
			Description:    "nose knows",
			Origin:         "upload",
			FingerprintHex: "beef",
			Size:           222,
			Timestamp:      time.Now(),
			Username:       "bambam",
		},
	})
	unitRev := unitRes.Revision()

	s.backend.EXPECT().Export(map[string]string{}).Return(s.model, nil)

	serialized, err := s.mustMakeAPI(c).Export()
	c.Assert(err, jc.ErrorIsNil)

	// We don't want to tie this test the serialisation output (that's
	// tested elsewhere). Just check that at least one thing we expect
	// is in the serialised output.
	c.Check(string(serialized.Bytes), jc.Contains, jujuversion.Current.String())

	c.Check(serialized.Charms, gc.DeepEquals, []string{"ch:foo-0"})
	if modelType == "caas" {
		c.Check(serialized.Tools, gc.HasLen, 0)
	} else {
		c.Check(serialized.Tools, jc.SameContents, []params.SerializedModelTools{
			{tools0, "/tools/" + tools0},
			{tools1, "/tools/" + tools1},
		})
	}
	c.Check(serialized.Resources, gc.DeepEquals, []params.SerializedModelResource{{
		Application: "foo",
		Name:        "bin",
		ApplicationRevision: params.SerializedModelResourceRevision{
			Revision:       appRev.Revision(),
			Type:           appRev.Type(),
			Path:           appRev.Path(),
			Description:    appRev.Description(),
			Origin:         appRev.Origin(),
			FingerprintHex: appRev.FingerprintHex(),
			Size:           appRev.Size(),
			Timestamp:      appRev.Timestamp(),
			Username:       appRev.Username(),
		},
		CharmStoreRevision: params.SerializedModelResourceRevision{
			Revision:       csRev.Revision(),
			Type:           csRev.Type(),
			Path:           csRev.Path(),
			Description:    csRev.Description(),
			Origin:         csRev.Origin(),
			FingerprintHex: csRev.FingerprintHex(),
			Size:           csRev.Size(),
			Timestamp:      csRev.Timestamp(),
			Username:       csRev.Username(),
		},
		UnitRevisions: map[string]params.SerializedModelResourceRevision{
			"foo/0": {
				Revision:       unitRev.Revision(),
				Type:           unitRev.Type(),
				Path:           unitRev.Path(),
				Description:    unitRev.Description(),
				Origin:         unitRev.Origin(),
				FingerprintHex: unitRev.FingerprintHex(),
				Size:           unitRev.Size(),
				Timestamp:      unitRev.Timestamp(),
				Username:       unitRev.Username(),
			},
		},
	}})
}

func (s *Suite) TestReap(c *gc.C) {
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

	err := s.mustMakeAPI(c).Reap()
	c.Check(err, jc.ErrorIsNil)

}

func (s *Suite) TestReapError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mig := mocks.NewMockModelMigration(ctrl)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)
	s.backend.EXPECT().RemoveExportingModelDocs().Return(errors.New("boom"))

	err := s.mustMakeAPI(c).Reap()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestWatchMinionReports(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Watcher with an initial event in the pipe.
	w := mocks.NewMockNotifyWatcher(ctrl)
	w.EXPECT().Stop().Return(nil).AnyTimes()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w.EXPECT().Changes().Return(ch).Times(2)

	mig := mocks.NewMockModelMigration(ctrl)
	mig.EXPECT().WatchMinionReports().Return(w, nil)

	s.backend.EXPECT().LatestMigration().Return(mig, nil)

	result := s.mustMakeAPI(c).WatchMinionReports()
	c.Assert(result.Error, gc.IsNil)

	resource := s.resources.Get(result.NotifyWatcherId)
	watcher, _ := resource.(state.NotifyWatcher)
	c.Assert(watcher, gc.NotNil)

	select {
	case <-watcher.Changes():
		c.Fatalf("initial event not consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *Suite) TestMinionReports(c *gc.C) {
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

	reports, err := s.mustMakeAPI(c).MinionReports()
	c.Assert(err, jc.ErrorIsNil)

	// Expect the sample of unknowns to be in order and be limited to
	// the first 10.
	expectedSample := make([]string, 0, 10)
	for i := 0; i < cap(expectedSample); i++ {
		expectedSample = append(expectedSample, names.NewMachineTag(fmt.Sprintf("%d", i)).String())
	}
	c.Assert(reports, gc.DeepEquals, params.MinionReports{
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

func (s *Suite) TestMinionReportTimeout(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	timeout := "30s"

	s.backend.EXPECT().ControllerConfig().Return(controller.Config{
		controller.MigrationMinionWaitMax: timeout,
	}, nil)

	res, err := s.mustMakeAPI(c).MinionReportTimeout()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Error, gc.IsNil)
	c.Check(res.Result, gc.Equals, timeout)
}

func (s *Suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerBackend = mocks.NewMockControllerState(ctrl)
	s.backend = mocks.NewMockBackend(ctrl)
	s.precheckBackend = mocks.NewMockPrecheckBackend(ctrl)
	return ctrl
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationmaster.API {
	api, err := s.makeAPI()
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) makeAPI() (*migrationmaster.API, error) {
	return migrationmaster.NewAPI(
		s.controllerBackend,
		s.backend,
		s.precheckBackend,
		nil, // pool
		s.resources,
		s.authorizer,
		&stubPresence{},
		func(names.ModelTag) (environscloudspec.CloudSpec, error) { return s.cloudSpec, nil },
		stubLeadership{},
	)
}

type stubPresence struct{}

func (f *stubPresence) ModelPresence(modelUUID string) facade.ModelPresence {
	return f
}

func (f *stubPresence) AgentStatus(agent string) (presence.Status, error) {
	return presence.Alive, nil
}

type stubLeadership struct{}

func (stubLeadership) Leaders() (map[string]string, error) {
	return map[string]string{}, nil
}
