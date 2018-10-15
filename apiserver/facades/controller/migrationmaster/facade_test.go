// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"fmt"
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type Suite struct {
	coretesting.BaseSuite

	model      description.Model
	stub       *testing.Stub
	backend    *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.model = description.NewModel(description.ModelArgs{
		Type:               "iaas",
		Config:             map[string]interface{}{"uuid": modelUUID},
		Owner:              names.NewUserTag("admin"),
		LatestToolsVersion: jujuversion.Current,
	})
	s.stub = new(testing.Stub)
	s.backend = &stubBackend{
		migration: &stubMigration{stub: s.stub},
		stub:      s.stub,
		model:     s.model,
	}

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
}

func (s *Suite) TestNotController(c *gc.C) {
	s.authorizer.Controller = false

	api, err := s.makeAPI()
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *Suite) TestWatch(c *gc.C) {
	api := s.mustMakeAPI(c)

	result := api.Watch()
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
	var expectedMacaroons = `
[[{"caveats":[],"location":"location","identifier":"id","signature":"a9802bf274262733d6283a69c62805b5668dbf475bcd7edc25a962833f7c2cba"}]]`[1:]

	api := s.mustMakeAPI(c)
	status, err := api.MigrationStatus()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status, gc.DeepEquals, params.MasterMigrationStatus{
		Spec: params.MigrationSpec{
			ModelTag: names.NewModelTag(modelUUID).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(controllerUUID).String(),
				Addrs:         []string{"1.1.1.1:1", "2.2.2.2:2"},
				CACert:        "trust me",
				AuthTag:       names.NewUserTag("admin").String(),
				Password:      "secret",
				Macaroons:     expectedMacaroons,
			},
		},
		MigrationId:      "id",
		Phase:            "IMPORT",
		PhaseChangedTime: s.backend.migration.PhaseChangedTime(),
	})
}

func (s *Suite) TestModelInfo(c *gc.C) {
	api := s.mustMakeAPI(c)
	model, err := api.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.UUID, gc.Equals, "model-uuid")
	c.Assert(model.Name, gc.Equals, "model-name")
	c.Assert(model.OwnerTag, gc.Equals, names.NewUserTag("owner").String())
	c.Assert(model.AgentVersion, gc.Equals, version.MustParse("1.2.3"))
}

func (s *Suite) TestSetPhase(c *gc.C) {
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.backend.migration.phaseSet, gc.Equals, coremigration.ABORT)
}

func (s *Suite) TestSetPhaseNoMigration(c *gc.C) {
	s.backend.getErr = errors.New("boom")
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetPhaseBadPhase(c *gc.C) {
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "wat"})
	c.Assert(err, gc.ErrorMatches, `invalid phase: "wat"`)
}

func (s *Suite) TestSetPhaseError(c *gc.C) {
	s.backend.migration.setPhaseErr = errors.New("blam")
	api := s.mustMakeAPI(c)

	err := api.SetPhase(params.SetMigrationPhaseArgs{Phase: "ABORT"})
	c.Assert(err, gc.ErrorMatches, "failed to set phase: blam")
}

func (s *Suite) TestSetStatusMessage(c *gc.C) {
	api := s.mustMakeAPI(c)

	err := api.SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.backend.migration.messageSet, gc.Equals, "foo")
}

func (s *Suite) TestSetStatusMessageNoMigration(c *gc.C) {
	s.backend.getErr = errors.New("boom")
	api := s.mustMakeAPI(c)

	err := api.SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Check(err, gc.ErrorMatches, "could not get migration: boom")
}

func (s *Suite) TestSetStatusMessageError(c *gc.C) {
	s.backend.migration.setMessageErr = errors.New("blam")
	api := s.mustMakeAPI(c)

	err := api.SetStatusMessage(params.SetMigrationStatusMessageArgs{Message: "foo"})
	c.Assert(err, gc.ErrorMatches, "failed to set status message: blam")
}

func (s *Suite) TestPrechecks(c *gc.C) {
	api := s.mustMakeAPI(c)
	err := api.Prechecks()
	c.Assert(err, gc.ErrorMatches, "retrieving model: boom")
}

func (s *Suite) TestExportIAAS(c *gc.C) {
	s.assertExport(c, "iaas")
}

func (s *Suite) TestExportCAAS(c *gc.C) {
	s.model = description.NewModel(description.ModelArgs{
		Type:               "caas",
		Config:             map[string]interface{}{"uuid": modelUUID},
		Owner:              names.NewUserTag("admin"),
		LatestToolsVersion: jujuversion.Current,
	})
	s.backend.model = s.model
	s.assertExport(c, "caas")
}

func (s *Suite) assertExport(c *gc.C, modelType string) {
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

	res := app.AddResource(description.ResourceArgs{"bin"})
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

	api := s.mustMakeAPI(c)
	serialized, err := api.Export()
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
	api := s.mustMakeAPI(c)
	s.backend.migration = &stubMigration{}

	err := api.Reap()
	c.Check(err, jc.ErrorIsNil)
	// Reaping should set the migration phase to DONE - otherwise
	// there's a race between the migrationmaster worker updating the
	// phase and being stopped because the model's gone. This leaves
	// the migration as active in the source controller, which will
	// prevent the model from being migrated back.
	s.backend.stub.CheckCalls(c, []testing.StubCall{
		{"LatestMigration", []interface{}{}},
		{"RemoveExportingModelDocs", []interface{}{}},
	})
	c.Assert(s.backend.migration.phaseSet, gc.Equals, coremigration.DONE)
}

func (s *Suite) TestReapError(c *gc.C) {
	s.backend.removeErr = errors.New("boom")
	api := s.mustMakeAPI(c)

	err := api.Reap()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestWatchMinionReports(c *gc.C) {
	api := s.mustMakeAPI(c)

	result := api.WatchMinionReports()
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c,
		"LatestMigration",
		"ModelMigration.WatchMinionReports",
	)

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
	// Report 16 unknowns. These are in reverse order in order to test
	// sorting.
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
	s.backend.migration.minionReports = &state.MinionReports{
		Succeeded: []names.Tag{m50, m51, u0},
		Failed:    []names.Tag{u1, m52, m50c1, m50c0},
		Unknown:   unknown,
	}

	api := s.mustMakeAPI(c)
	reports, err := api.MinionReports()
	c.Assert(err, jc.ErrorIsNil)

	// Expect the sample of unknowns to be in order and be limited to
	// the first 10.
	expectedSample := make([]string, 0, 10)
	for i := 0; i < cap(expectedSample); i++ {
		expectedSample = append(expectedSample, names.NewMachineTag(fmt.Sprintf("%d", i)).String())
	}
	c.Assert(reports, gc.DeepEquals, params.MinionReports{
		MigrationId:   "id",
		Phase:         "IMPORT",
		SuccessCount:  3,
		UnknownCount:  len(unknown),
		UnknownSample: expectedSample,
		Failed: []string{
			// Note sorting
			m50c0.String(),
			m50c1.String(),
			m52.String(),
			u1.String(),
		},
	})
}

func (s *Suite) makeAPI() (*migrationmaster.API, error) {
	return migrationmaster.NewAPI(
		s.backend,
		new(failingPrecheckBackend),
		nil, // pool
		s.resources,
		s.authorizer,
		&fakePresence{},
	)
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationmaster.API {
	api, err := s.makeAPI()
	c.Assert(err, jc.ErrorIsNil)
	return api
}

type stubBackend struct {
	migrationmaster.Backend

	stub      *testing.Stub
	getErr    error
	removeErr error
	migration *stubMigration
	model     description.Model
}

func (b *stubBackend) WatchForMigration() state.NotifyWatcher {
	b.stub.AddCall("WatchForMigration")
	return apiservertesting.NewFakeNotifyWatcher()
}

func (b *stubBackend) LatestMigration() (state.ModelMigration, error) {
	b.stub.AddCall("LatestMigration")
	if b.getErr != nil {
		return nil, b.getErr
	}
	return b.migration, nil
}

func (b *stubBackend) ModelUUID() string {
	return "model-uuid"
}

func (b *stubBackend) ModelName() (string, error) {
	return "model-name", nil
}

func (b *stubBackend) ModelOwner() (names.UserTag, error) {
	return names.NewUserTag("owner"), nil
}

func (b *stubBackend) AgentVersion() (version.Number, error) {
	return version.MustParse("1.2.3"), nil
}

func (b *stubBackend) RemoveExportingModelDocs() error {
	b.stub.AddCall("RemoveExportingModelDocs")
	return b.removeErr
}

func (b *stubBackend) Export() (description.Model, error) {
	b.stub.AddCall("Export")
	return b.model, nil
}

type stubMigration struct {
	state.ModelMigration

	stub            *testing.Stub
	setPhaseErr     error
	phaseSet        coremigration.Phase
	setMessageErr   error
	messageSet      string
	minionReports   *state.MinionReports
	externalControl bool
}

func (m *stubMigration) Id() string {
	return "id"
}

func (m *stubMigration) Phase() (coremigration.Phase, error) {
	return coremigration.IMPORT, nil
}

func (m *stubMigration) PhaseChangedTime() time.Time {
	return time.Date(2016, 6, 22, 16, 38, 0, 0, time.UTC)
}

func (m *stubMigration) ModelUUID() string {
	return modelUUID
}

func (m *stubMigration) TargetInfo() (*coremigration.TargetInfo, error) {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location")
	if err != nil {
		panic(err)
	}
	return &coremigration.TargetInfo{
		ControllerTag: names.NewControllerTag(controllerUUID),
		Addrs:         []string{"1.1.1.1:1", "2.2.2.2:2"},
		CACert:        "trust me",
		AuthTag:       names.NewUserTag("admin"),
		Password:      "secret",
		Macaroons:     []macaroon.Slice{{mac}},
	}, nil
}

func (m *stubMigration) SetPhase(phase coremigration.Phase) error {
	if m.setPhaseErr != nil {
		return m.setPhaseErr
	}
	m.phaseSet = phase
	return nil
}

func (m *stubMigration) SetStatusMessage(message string) error {
	if m.setMessageErr != nil {
		return m.setMessageErr
	}
	m.messageSet = message
	return nil
}

func (m *stubMigration) WatchMinionReports() (state.NotifyWatcher, error) {
	m.stub.AddCall("ModelMigration.WatchMinionReports")
	return apiservertesting.NewFakeNotifyWatcher(), nil
}

func (m *stubMigration) MinionReports() (*state.MinionReports, error) {
	return m.minionReports, nil
}

var modelUUID string
var controllerUUID string

func init() {
	modelUUID = utils.MustNewUUID().String()
	controllerUUID = utils.MustNewUUID().String()
}

type failingPrecheckBackend struct {
	migration.PrecheckBackend
}

func (b *failingPrecheckBackend) Model() (migration.PrecheckModel, error) {
	return nil, errors.New("boom")
}

type fakePresence struct {
}

func (f *fakePresence) ModelPresence(modelUUID string) facade.ModelPresence {
	return f
}

func (f *fakePresence) AgentStatus(agent string) (presence.Status, error) {
	return presence.Alive, nil
}
