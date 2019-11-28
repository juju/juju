// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/description"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type Suite struct {
	coretesting.BaseSuite

	backend         *mocks.MockBackend
	precheckBackend *mocks.MockPrecheckBackend

	controllerUUID string
	modelUUID      string
	model          description.Model
	resources      *common.Resources
	authorizer     apiservertesting.FakeAuthorizer
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
}

func (s *Suite) TestNotController(c *gc.C) {
	s.authorizer.Controller = false

	api, err := s.makeAPI()
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.Equals, common.ErrPerm)
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

	mig := mocks.NewMockModelMigration(ctrl)

	mac, err := macaroon.New([]byte(password), []byte("id"), "location")
	c.Assert(err, jc.ErrorIsNil)

	targetInfo := coremigration.TargetInfo{
		ControllerTag: names.NewControllerTag(s.controllerUUID),
		Addrs:         []string{"1.1.1.1:1", "2.2.2.2:2"},
		CACert:        "trust me",
		AuthTag:       names.NewUserTag("admin"),
		Password:      password,
		Macaroons:     []macaroon.Slice{{mac}},
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
				Macaroons: `[[{"caveats":[],"location":"location","identifier":"id",` +
					`"signature":"a9802bf274262733d6283a69c62805b5668dbf475bcd7edc25a962833f7c2cba"}]]`,
			},
		},
		MigrationId:      "ID",
		Phase:            "IMPORT",
		PhaseChangedTime: now,
	})
}

func (s *Suite) TestModelInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.backend.EXPECT()
	exp.ModelUUID().Return("model-uuid")
	exp.ModelName().Return("model-name", nil)
	exp.ModelOwner().Return(names.NewUserTag("owner"), nil)
	exp.AgentVersion().Return(version.MustParse("1.2.3"), nil)

	mod, err := s.mustMakeAPI(c).ModelInfo()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mod.UUID, gc.Equals, "model-uuid")
	c.Assert(mod.Name, gc.Equals, "model-name")
	c.Assert(mod.OwnerTag, gc.Equals, names.NewUserTag("owner").String())
	c.Assert(mod.AgentVersion, gc.Equals, version.MustParse("1.2.3"))
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

	err := s.mustMakeAPI(c).Prechecks()
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
		CharmURL: "cs:foo-0",
	})

	const tools0 = "2.0.0-xenial-amd64"
	const tools1 = "2.0.1-xenial-amd64"
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

	s.backend.EXPECT().Export().Return(s.model, nil)

	serialized, err := s.mustMakeAPI(c).Export()
	c.Assert(err, jc.ErrorIsNil)

	// We don't want to tie this test the serialisation output (that's
	// tested elsewhere). Just check that at least one thing we expect
	// is in the serialised output.
	c.Check(string(serialized.Bytes), jc.Contains, jujuversion.Current.String())

	c.Check(serialized.Charms, gc.DeepEquals, []string{"cs:foo-0"})
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

func (s *Suite) TestPrechecksCrossModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	user := "some-random-cat"

	// These UUIDs are used for both the model and the offer.
	id1 := utils.MustNewUUID().String()
	id2 := utils.MustNewUUID().String()
	id3 := utils.MustNewUUID().String()

	c1 := mocks.NewMockOfferConnection(ctrl)
	c1Exp := c1.EXPECT()
	c1Exp.SourceModelUUID().Return(id1)
	c1Exp.UserName().Return(user)
	c1Exp.RelationId().Return(1)
	c1Exp.OfferUUID().Return(id1)

	c2 := mocks.NewMockOfferConnection(ctrl)
	c2Exp := c2.EXPECT()
	c2Exp.SourceModelUUID().Return(id2)
	c2Exp.UserName().Return(user)
	c2Exp.RelationId().Return(2)
	c2Exp.OfferUUID().Return(id2)

	c3 := mocks.NewMockOfferConnection(ctrl)
	c3Exp := c3.EXPECT()
	c3Exp.SourceModelUUID().Return(id3)
	c3Exp.UserName().Return(user)
	c3Exp.RelationId().Return(3)
	c3Exp.OfferUUID().Return(id3)

	bExp := s.backend.EXPECT()
	bExp.AllOfferConnections().Return([]migrationmaster.OfferConnection{c1, c2, c3}, nil)

	// m1 will be a model in the same controller.
	// m2 and m3 will be on an external controller.
	cont := mocks.NewMockExternalController(ctrl)
	cID := utils.MustNewUUID().String()

	cont.EXPECT().Id().Return(cID).Times(2)
	cont.EXPECT().ControllerInfo().Return(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(cID),
		Alias:         "controller-alias",
		Addrs:         []string{"1.1.1.1:1"},
		CACert:        "some-cert-string",
	}).Times(2)

	bExp.ControllerForModel(id1).Return(nil, errors.NotFoundf(""))
	bExp.ControllerForModel(id2).Return(cont, nil)
	bExp.ControllerForModel(id3).Return(cont, nil)

	result, err := s.mustMakeAPI(c).PrechecksCrossModel()
	c.Assert(err, jc.ErrorIsNil)

	expectedCInfo := params.ExternalControllerInfo{
		ControllerTag: names.NewControllerTag(cID).String(),
		Alias:         "controller-alias",
		Addrs:         []string{"1.1.1.1:1"},
		CACert:        "some-cert-string",
	}
	expectedResult := params.MigratingCrossModelResult{
		OfferConnections: []params.MigratingOfferConnection{
			{
				OfferUUID:      id1,
				SourceModelTag: names.NewModelTag(id1).String(),
				RelationID:     1,
				UserName:       user,
			},
			{
				OfferUUID:      id2,
				SourceModelTag: names.NewModelTag(id2).String(),
				RelationID:     2,
				UserName:       user,
			},
			{
				OfferUUID:      id3,
				SourceModelTag: names.NewModelTag(id3).String(),
				RelationID:     3,
				UserName:       user,
			},
		},
		ExternalControllers: map[string]params.ExternalControllerInfo{
			names.NewModelTag(id1).String(): {},
			names.NewModelTag(id2).String(): expectedCInfo,
			names.NewModelTag(id3).String(): expectedCInfo,
		},
		Error: nil,
	}
	c.Check(result, jc.DeepEquals, expectedResult)
}

func (s *Suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

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
		s.backend,
		s.precheckBackend,
		nil, // pool
		s.resources,
		s.authorizer,
		&stubPresence{},
	)
}

type stubPresence struct{}

func (f *stubPresence) ModelPresence(modelUUID string) facade.ModelPresence {
	return f
}

func (f *stubPresence) AgentStatus(agent string) (presence.Status, error) {
	return presence.Alive, nil
}
