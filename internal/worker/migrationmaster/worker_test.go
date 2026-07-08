// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"context"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/macaroon.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	coreresource "github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	domainexport "github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/rpc/params"
)

type Suite struct {
	coretesting.BaseSuite
	clock                   *testclock.Clock
	stub                    *testhelpers.Stub
	connection              *stubConnection
	connectionErr           error
	facade                  *stubMasterFacade
	modelMigrationService   *stubModelMigrationService
	exportService           *stubExportService
	controllerConfigService *stubControllerConfigService
	modelAgentService       *stubModelAgentService
	resourceService         *stubResourceService
	charmService            *stubCharmService
	loggingService          *stubLoggingService
	config                  migrationmaster.Config
}

func TestSuite(t *testing.T) {
	tc.Run(t, &Suite{})
}

var (
	sourceControllerTag = names.NewControllerTag("source-controller-uuid")
	targetControllerTag = names.NewControllerTag("controller-uuid")
	modelUUID           = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
	modelTag            = names.NewModelTag(modelUUID)
	modelName           = "model-name"
	modelQualifier      = model.Qualifier("prod")

	fakeExportVersion = semversion.MustParse("4.0.6")
	fakeExportPayload = map[string]string{"model": "data"}

	fakeCharmLocators = []applicationcharm.CharmLocator{{
		Name:         "charm0",
		Revision:     0,
		Source:       applicationcharm.CharmHubSource,
		Architecture: architecture.AMD64,
	}, {
		Name:         "charm1",
		Revision:     1,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.Unknown,
	}}
	fakeCharmURLs = []string{"ch:amd64/charm0-0", "local:charm1-1"}

	fakeToolsSHA256  = "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"
	fakeMachineTools = map[machine.Name]coreagentbinary.Metadata{
		"0": {
			Version: coreagentbinary.Version{
				Number: semversion.MustParse("2.1.0"),
				Arch:   arch.AMD64,
			},
			SHA256: fakeToolsSHA256,
		},
	}
	fakeUploadTools = map[string]semversion.Binary{
		fakeToolsSHA256: semversion.MustParseBinary("2.1.0-ubuntu-amd64"),
	}

	fakeControllerModelInfo = coremodelmigration.ControllerModelInfo{
		ModelInfo: coremodelmigration.ModelIdentityInfo{
			UUID:      modelUUID,
			Name:      modelName,
			Qualifier: modelQualifier.String(),
			Type:      "iaas",
			Cloud:     "aws",
			Life:      "alive",
		},
	}

	// Define stub calls that commonly appear in tests here to allow reuse.
	apiOpenControllerCall = testhelpers.StubCall{
		FuncName: "apiOpen",
		Args: []any{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
			},
			migration.ControllerDialOpts(nil),
		},
	}
	activateCall = testhelpers.StubCall{
		FuncName: "MigrationTarget.Activate",
		Args: []any{
			params.ActivateModelArgs{
				ModelTag: modelTag.String(),
			},
		},
	}
	checkMachinesCall = testhelpers.StubCall{
		FuncName: "MigrationTarget.CheckMachines",
		Args: []any{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	adoptResourcesCall = testhelpers.StubCall{
		FuncName: "MigrationTarget.AdoptResources",
		Args: []any{
			params.AdoptResourcesArgs{
				ModelTag:                modelTag.String(),
				SourceControllerVersion: jujuversion.Current,
			},
		},
	}
	latestLogTimeCall = testhelpers.StubCall{
		FuncName: "MigrationTarget.LatestLogTime",
		Args: []any{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	apiCloseCall      = testhelpers.StubCall{FuncName: "Connection.Close"}
	getLokiConfigCall = testhelpers.StubCall{FuncName: "loggingService.IsLokiEnabled"}
	abortCall         = testhelpers.StubCall{
		FuncName: "MigrationTarget.Abort",
		Args: []any{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	watchStatusLockdownCalls = []testhelpers.StubCall{
		{FuncName: "modelMigrationService.WatchForMigration", Args: nil},
		{FuncName: "modelMigrationService.Migration", Args: nil},
		{FuncName: "guard.Lockdown", Args: nil},
	}
	// assembleCalls are the service calls recorded by one envelope assembly.
	assembleCalls = []testhelpers.StubCall{
		{FuncName: "exportService.Export", Args: nil},
		{FuncName: "exportService.GetControllerModelInfo", Args: nil},
		{FuncName: "charmService.ListCharmLocators", Args: nil},
		{FuncName: "modelAgentService.GetModelAgentBinaryMetadata", Args: nil},
		{FuncName: "resourceService.ListAllModelResources", Args: nil},
	}
	abortCalls = []testhelpers.StubCall{
		{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORT}},
		apiOpenControllerCall,
		abortCall,
		apiCloseCall,
		{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORTDONE}},
	}
	openDestLogStreamCall = testhelpers.StubCall{FuncName: "ConnectControllerStream", Args: []any{
		"/migrate/logtransfer",
		url.Values{},
		http.Header{
			textproto.CanonicalMIMEHeaderKey(params.MigrationModelHTTPHeader): {modelUUID},
		},
	}}
)

func (s *Suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Now())
	s.stub = new(testhelpers.Stub)
	s.connection = &stubConnection{
		c:             c,
		stub:          s.stub,
		controllerTag: targetControllerTag,
		logStream:     &mockStream{},
		controllerVersion: params.ControllerVersionResults{
			Version: "2.9.99",
		},
		facadeVersion: 8,
	}
	s.connectionErr = nil

	s.facade = newStubMasterFacade(s.stub)
	s.modelMigrationService = newStubModelMigrationService(s.stub)
	s.exportService = &stubExportService{stub: s.stub}
	s.controllerConfigService = &stubControllerConfigService{stub: s.stub}
	s.modelAgentService = &stubModelAgentService{stub: s.stub}
	s.resourceService = &stubResourceService{stub: s.stub}
	s.charmService = &stubCharmService{stub: s.stub}
	s.loggingService = &stubLoggingService{stub: s.stub}

	// The default worker Config used by most of the tests. Tests may
	// tweak parts of this as needed.
	s.config = migrationmaster.Config{
		ModelUUID:               modelUUID,
		CharmService:            s.charmService,
		ModelMigrationService:   s.modelMigrationService,
		ExportService:           s.exportService,
		ControllerConfigService: s.controllerConfigService,
		ModelAgentService:       s.modelAgentService,
		ResourceService:         s.resourceService,
		Guard:                   newStubGuard(s.stub),
		APIOpen:                 s.apiOpen,
		UploadBinaries:          nullUploadBinaries,
		AgentBinaryStore:        fakeAgentBinaryStore,
		LoggingService:          s.loggingService,
		Clock:                   s.clock,
		SourcePrecheck:          s.facade.Prechecks,
		StreamModelLog:          s.facade.StreamModelLog,
	}
}

// expectedEnvelope builds the SerializedModelV2 envelope the worker is
// expected to assemble from the suite's stub services.
func (s *Suite) expectedEnvelope(c *tc.C) params.SerializedModelV2 {
	payload, err := goyaml.Marshal(fakeExportPayload)
	c.Assert(err, tc.ErrorIsNil)
	envelope := migrationmaster.EnvelopeFromControllerModelInfo(fakeControllerModelInfo, "model-uuid:2")
	envelope.PayloadVersion = fakeExportVersion
	envelope.Payload = payload
	envelope.Charms = fakeCharmURLs
	_, envelope.Tools = migrationmaster.ToolsForEnvelope(fakeMachineTools, nil)
	envelope.Resources = migrationmaster.ResourcesForEnvelope(s.resourceService.resources)
	return envelope
}

// prechecksCalls are the stub calls recorded by one worker prechecks pass.
func (s *Suite) prechecksCalls(c *tc.C) []testhelpers.StubCall {
	return joinCalls(
		[]testhelpers.StubCall{
			{FuncName: "facade.Prechecks", Args: []any{}},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			{FuncName: "MigrationTarget.Prechecks", Args: []any{s.expectedEnvelope(c)}},
			apiCloseCall,
		},
	)
}

// importCall is the v8 import of the authoritative envelope.
func (s *Suite) importCall(c *tc.C) testhelpers.StubCall {
	return testhelpers.StubCall{
		FuncName: "MigrationTarget.Import",
		Args:     []any{s.expectedEnvelope(c)},
	}
}

func (s *Suite) apiOpen(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
	s.stub.AddCall("apiOpen", info, dialOpts)
	if s.connectionErr != nil {
		return nil, s.connectionErr
	}
	return s.connection, nil
}

func (s *Suite) makeStatus(phase coremigration.Phase) coremigration.MigrationStatus {
	return coremigration.MigrationStatus{
		MigrationId:      "model-uuid:2",
		ModelUUID:        "model-uuid",
		Phase:            phase,
		PhaseChangedTime: s.clock.Now(),
		TargetInfo: coremigration.TargetInfo{
			ControllerUUID: targetControllerTag.Id(),
			Addrs:          []string{"1.2.3.4:5"},
			CACert:         "cert",
			User:           "admin",
			Password:       "secret",
		},
	}
}

func (s *Suite) TestSuccessfulMigration(c *tc.C) {
	s.resourceService.resources = []coreresource.Resource{
		resourcetesting.NewResource(c, nil, "blob", "app", "").Resource,
	}

	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.QUIESCE))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.SUCCESS))
	s.config.UploadBinaries = makeStubUploadBinaries(s.stub)

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	assertExpectedCallArgs(c, s.stub, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},

		// QUIESCE
		s.prechecksCalls(c),
		[]testhelpers.StubCall{
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
		},
		s.prechecksCalls(c),
		[]testhelpers.StubCall{
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.IMPORT}},
		},

		//IMPORT
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			s.importCall(c),
			{FuncName: "UploadBinaries", Args: []any{
				fakeCharmURLs,
				s.charmService,
				fakeUploadTools,
				fakeAgentBinaryStore,
				s.resourceService.resources,
				s.facade,
			}},
			apiCloseCall, // for target controller
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.VALIDATION}},

			// VALIDATION
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			{FuncName: "modelMigrationService.SourceControllerInfo", Args: nil},
			activateCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.SUCCESS}},

			// SUCCESS
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.LOGTRANSFER}},

			// LOGTRANSFER
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},

			// REAP
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		}),
	)
}

func (s *Suite) TestIncompatibleTarget(c *tc.C) {
	// The target controller only offers the legacy migrationtarget
	// facade (v7); the new envelope path requires v8 and hard-errors.
	s.connection.facadeVersion = 7

	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},

		// QUIESCE
		[]testhelpers.StubCall{
			{FuncName: "facade.Prechecks", Args: []any{}},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			apiCloseCall,
		},
		abortCalls,
	))
	lastMessages := s.modelMigrationService.statuses[len(s.modelMigrationService.statuses)-2:]
	c.Assert(lastMessages[0], tc.Matches,
		"(?s)target controller does not support the model migration format.*")
}

func (s *Suite) TestMigrationResume(c *tc.C) {
	// Test that a partially complete migration can be resumed.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.SUCCESS))

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.LOGTRANSFER}},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
}

func (s *Suite) TestPreviouslyAbortedMigration(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.ABORTDONE))

	w, err := migrationmaster.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"modelMigrationService.WatchForMigration",
		"modelMigrationService.Migration",
		"guard.Unlock",
	})
}

func (s *Suite) TestPreviouslyCompletedMigration(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.DONE))
	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "modelMigrationService.WatchForMigration", Args: nil},
		{FuncName: "modelMigrationService.Migration", Args: nil},
	})
}

func (s *Suite) TestWatchFailure(c *tc.C) {
	s.modelMigrationService.watchErr = errors.New("boom")
	s.checkWorkerErr(c, "watching for migration: boom")
}

func (s *Suite) TestStatusError(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.modelMigrationService.statusErr = errors.New("splat")

	s.checkWorkerErr(c, "retrieving migration status: splat")
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "modelMigrationService.WatchForMigration", Args: nil},
		{FuncName: "modelMigrationService.Migration", Args: nil},
	})
}

func (s *Suite) TestStatusNone(c *tc.C) {
	// A migration with phase NONE means the model has never been
	// migrated: the worker keeps waiting with the fortress unlocked.
	s.modelMigrationService.queueStatus(coremigration.MigrationStatus{Phase: coremigration.NONE})

	w, err := migrationmaster.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"modelMigrationService.WatchForMigration",
		"modelMigrationService.Migration",
		"guard.Unlock",
	})
}

func (s *Suite) TestUnlockError(c *tc.C) {
	s.modelMigrationService.queueStatus(coremigration.MigrationStatus{Phase: coremigration.NONE})
	guard := newStubGuard(s.stub)
	guard.unlockErr = errors.New("pow")
	s.config.Guard = guard

	s.checkWorkerErr(c, "pow")
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "modelMigrationService.WatchForMigration", Args: nil},
		{FuncName: "modelMigrationService.Migration", Args: nil},
		{FuncName: "guard.Unlock", Args: nil},
	})
}

func (s *Suite) TestLockdownError(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	guard := newStubGuard(s.stub)
	guard.lockdownErr = errors.New("biff")
	s.config.Guard = guard

	s.checkWorkerErr(c, "biff")
	s.stub.CheckCalls(c, watchStatusLockdownCalls)
}

func (s *Suite) TestQUIESCEMinionWaitWatchError(c *tc.C) {
	s.checkMinionWaitWatchError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEMinionWaitGetError(c *tc.C) {
	s.checkMinionWaitGetError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEFailedAgent(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.QUIESCE,
		TotalCount:     1,
		FailedMachines: []string{"42"}, // a machine failed
	})

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)

	expectedCalls := joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		s.prechecksCalls(c),
		[]testhelpers.StubCall{
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
		},
		abortCalls,
	)

	assertExpectedCallArgs(c, s.stub, expectedCalls)
}

func (s *Suite) TestQUIESCEWrongController(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.controllerTag = names.NewControllerTag("another-controller")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "facade.Prechecks", Args: []any{}},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCESourceChecksFail(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.prechecksErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "facade.Prechecks", Args: []any{}},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCEControllerModelInfoFail(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.exportService.controllerModelInfoErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "facade.Prechecks", Args: []any{}},
			{FuncName: "exportService.Export", Args: nil},
			{FuncName: "exportService.GetControllerModelInfo", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCETargetChecksFail(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.prechecksErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	assertExpectedCallArgs(c, s.stub, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		s.prechecksCalls(c),
		abortCalls,
	))
}

func (s *Suite) TestExportFailure(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.exportService.exportErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "exportService.Export", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestAPIOpenFailure(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connectionErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORT}},
			apiOpenControllerCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestImportFailure(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connection.importErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			s.importCall(c),
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestImportFailureAlreadyExistsActivating(c *tc.C) {
	// Preserve the activation resume path until Import v8 exposes structured
	// target state for duplicate imports.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connection.importErr = &params.Error{
		Code:    params.CodeAlreadyExists,
		Message: "model import for " + modelUUID + ": activation in progress",
	}
	// Terminate the worker at the VALIDATION minion wait so the test
	// only exercises the IMPORT decision.
	s.modelMigrationService.minionReportsWatchErr = errors.New("boom")

	s.checkWorkerErr(c, "boom")
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			s.importCall(c),
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.VALIDATION}},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
		},
	))
}

func (s *Suite) TestImportFailureAlreadyExistsImporting(c *tc.C) {
	// The target reports the model UUID is occupied by another import
	// that has not started activating: the worker must abort.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connection.importErr = &params.Error{
		Code:    params.CodeAlreadyExists,
		Message: "model import for " + modelUUID,
	}

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			s.importCall(c),
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestIncompatibleTargetImport(c *tc.C) {
	// A v8-incapable target discovered at IMPORT (e.g. after a worker
	// restart skipped QUIESCE) is also a hard error and aborts.
	s.connection.facadeVersion = 7
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.IMPORT))

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
		},
		assembleCalls,
		[]testhelpers.StubCall{
			apiOpenControllerCall,
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestVALIDATIONMinionWaitWatchError(c *tc.C) {
	s.checkMinionWaitWatchError(c, coremigration.VALIDATION)
}

func (s *Suite) TestVALIDATIONMinionWaitGetError(c *tc.C) {
	s.checkMinionWaitGetError(c, coremigration.VALIDATION)
}

func (s *Suite) TestVALIDATIONFailedAgent(c *tc.C) {
	// Set the last phase change status to be further back
	// in time than the max wait time for minion reports.
	sts := s.makeStatus(coremigration.VALIDATION)
	sts.PhaseChangedTime = time.Now().Add(-20 * time.Minute)
	s.modelMigrationService.queueStatus(sts)

	w, err := migrationmaster.New(s.config)
	c.Assert(err, tc.ErrorIsNil)

	// Queue the reports *after* the watcher is started.
	// The test will only pass if the minion wait timeout
	// is independent of the phase change time.
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.VALIDATION,
		TotalCount:     1,
		FailedMachines: []string{"42"}, // a machine failed
	})

	err = workertest.CheckKilled(c, w)
	c.Check(errors.Cause(err), tc.Equals, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestVALIDATIONCheckMachinesOneError(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.VALIDATION))

	s.connection.machineErrs = []string{"been so strange"}
	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
		abortCalls,
	))
	lastMessages := s.modelMigrationService.statuses[len(s.modelMigrationService.statuses)-2:]
	c.Assert(lastMessages, tc.DeepEquals, []string{
		"machine sanity check failed, 1 error found",
		"aborted, removing model from target controller: machine sanity check failed, 1 error found",
	})
}

func (s *Suite) TestVALIDATIONCheckMachinesSeveralErrors(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.connection.machineErrs = []string{"been so strange", "lit up"}
	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
		abortCalls,
	))
	lastMessages := s.modelMigrationService.statuses[len(s.modelMigrationService.statuses)-2:]
	c.Assert(lastMessages, tc.DeepEquals, []string{
		"machine sanity check failed, 2 errors found",
		"aborted, removing model from target controller: machine sanity check failed, 2 errors found",
	})
}

func (s *Suite) TestVALIDATIONCheckMachinesOtherError(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.connection.checkMachineErr = errors.Errorf("something went bang")

	s.checkWorkerReturns(c, s.connection.checkMachineErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitWatchError(c *tc.C) {
	s.checkMinionWaitWatchError(c, coremigration.SUCCESS)
}

func (s *Suite) TestSUCCESSMinionWaitGetError(c *tc.C) {
	s.checkMinionWaitGetError(c, coremigration.SUCCESS)
}

func (s *Suite) TestSUCCESSMinionWaitFailedMachine(c *tc.C) {
	// With the SUCCESS phase the master should wait for all reports,
	// continuing even if some minions report failure.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.SUCCESS,
		TotalCount:     1,
		FailedMachines: []string{"42"},
	})

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.LOGTRANSFER}},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitFailedUnit(c *tc.C) {
	// See note for TestMinionWaitFailedMachine above.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId:        "model-uuid:2",
		Phase:              coremigration.SUCCESS,
		TotalCount:         2,
		FailedUnits:        []string{"foo/2"},
		FailedApplications: []string{"bar"},
	})

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			{FuncName: "modelMigrationService.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.LOGTRANSFER}},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitTimeout(c *tc.C) {
	// The SUCCESS phase is special in that even if some minions fail
	// to report the migration should continue. There's no turning
	// back from SUCCESS.
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))

	w, err := migrationmaster.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for clock.After call")
	}

	// Move time ahead in order to trigger timeout.
	s.clock.Advance(15 * time.Minute)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.Equals, migrationmaster.ErrMigrated)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{FuncName: "modelMigrationService.WatchMinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.LOGTRANSFER}},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
}

func (s *Suite) TestMinionWaitWrongPhase(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))

	// Have the phase in the minion reports be different from the
	// migration status. This shouldn't happen but the migrationmaster
	// should handle it.
	s.modelMigrationService.queueMinionReports(makeMinionReports(coremigration.IMPORT))

	s.checkWorkerErr(c,
		`minion reports phase \(IMPORT\) does not match migration phase \(SUCCESS\)`)
}

func (s *Suite) TestMinionWaitMigrationIdChanged(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))

	// Have the migration id in the minion reports be different from
	// the migration status. This shouldn't happen but the
	// migrationmaster should handle it.
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId: "blah",
		Phase:       coremigration.SUCCESS,
	})

	s.checkWorkerErr(c,
		"unexpected migration id in minion reports, got blah, expected model-uuid:2")
}

func (s *Suite) TestMinionWaitInvalidReportCounts(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.modelMigrationService.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.SUCCESS,
		TotalCount:     1,
		SuccessCount:   1,
		FailedMachines: []string{"42"},
	})

	err := s.runWorker(c)
	c.Check(err, tc.ErrorIs, coremigration.ErrMinionReportsInvalid)
}

func (s *Suite) assertAPIConnectWithMacaroon(c *tc.C, authUser names.UserTag) {
	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	status := s.makeStatus(coremigration.ABORT)
	status.TargetInfo.User = authUser.Id()

	// Set up macaroon based auth to the target.
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	macs := []macaroon.Slice{{mac}}
	status.TargetInfo.Password = ""
	status.TargetInfo.Macaroons = macs

	s.modelMigrationService.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	var apiUser names.Tag
	if authUser.IsLocal() {
		apiUser = authUser
	}
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{
				FuncName: "apiOpen",
				Args: []any{
					&api.Info{
						Addrs:     []string{"1.2.3.4:5"},
						CACert:    "cert",
						Tag:       apiUser,
						Macaroons: macs, // <---
					},
					migration.ControllerDialOpts(nil),
				},
			},
			abortCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestAPIConnectWithMacaroonLocalUser(c *tc.C) {
	s.assertAPIConnectWithMacaroon(c, names.NewUserTag("admin"))
}

func (s *Suite) TestAPIConnectWithMacaroonExternalUser(c *tc.C) {
	s.assertAPIConnectWithMacaroon(c, names.NewUserTag("fred@external"))
}

func (s *Suite) TestAPIConnectionWithToken(c *tc.C) {
	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	status := s.makeStatus(coremigration.ABORT)
	status.TargetInfo.User = "fred@external"

	// Set up token based auth to the target.
	status.TargetInfo.Password = ""
	status.TargetInfo.Macaroons = nil
	status.TargetInfo.Token = "token"

	s.modelMigrationService.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	expectedLoginProvider := api.NewSessionTokenLoginProvider("token", nil, nil)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			{
				FuncName: "apiOpen",
				Args: []any{
					&api.Info{
						Addrs:  []string{"1.2.3.4:5"},
						CACert: "cert",
						Tag:    nil,
					},
					migration.ControllerDialOpts(expectedLoginProvider),
				},
			},
			abortCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningTargetAPI(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connectionErr = errors.New("people of earth")

	s.checkWorkerReturns(c, s.connectionErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorGettingStartTime(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connection.latestLogErr = errors.New("tender vittles")

	s.checkWorkerReturns(c, s.connection.latestLogErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningLogSource(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.facade.streamErr = errors.New("chicken bones")

	s.checkWorkerReturns(c, s.facade.streamErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningLogDest(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connection.streamErr = errors.New("tule lake shuffle")

	s.checkWorkerReturns(c, s.connection.streamErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorWriting(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.facade.logMessages = func(d chan<- common.LogMessage) {
		safeSend(c, d, common.LogMessage{Message: "the go team"})
	}
	s.connection.logStream.writeErr = errors.New("bottle rocket")
	s.checkWorkerReturns(c, s.connection.logStream.writeErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
		},
	))
	c.Assert(s.connection.logStream.closeCount, tc.Equals, 1)
}

func (s *Suite) TestLogTransferSendsRecords(c *tc.C) {
	t1, err := time.Parse("2006-01-02 15:04", "2016-11-28 16:11")
	c.Assert(err, tc.ErrorIsNil)
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	messages := []common.LogMessage{
		{Message: "the go team"},
		{Message: "joan as police woman"},
		{
			Entity:    "the mules",
			Timestamp: t1,
			Severity:  "warning",
			Module:    "this one",
			Location:  "nearby",
			Message:   "ham shank",
		},
	}
	s.facade.logMessages = func(d chan<- common.LogMessage) {
		for _, message := range messages {
			safeSend(c, d, message)
		}
	}

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
	c.Assert(s.connection.logStream.written, tc.DeepEquals, []params.LogRecord{
		{Message: "the go team"},
		{Message: "joan as police woman"},
		{
			Time:     t1,
			Module:   "this one",
			Location: "nearby",
			Level:    "warning",
			Message:  "ham shank",
			Entity:   "the mules",
		},
	})
	c.Assert(s.connection.logStream.closeCount, tc.Equals, 1)
}

func (s *Suite) TestLogTransferReportsProgress(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	messages := []common.LogMessage{
		{Message: "captain beefheart"},
		{Message: "super furry animals"},
		{Message: "ezra furman"},
		{Message: "these new puritans"},
	}
	s.facade.logMessages = func(d chan<- common.LogMessage) {
		for _, message := range messages {
			safeSend(c, d, message)
			c.Assert(s.clock.WaitAdvance(20*time.Second, coretesting.LongWait, 1), tc.ErrorIsNil)
		}
	}

	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("migrationmaster-tests", &logWriter), tc.ErrorIsNil)
	defer func() {
		_, _ = loggo.RemoveWriter("migrationmaster-tests")
		logWriter.Clear()
	}()

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_]._`, tc.Ignore)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	c.Assert(logWriter.Log()[:3], mc, []loggo.Entry{
		{Message: "successful, transferring logs to target controller \\(0 sent\\)"},
		// This is a bit of a punt, but without accepting a range
		// we sometimes see this test failing on loaded test machines.
		{Message: "successful, transferring logs to target controller \\([23] sent\\)"},
		{Message: "successful, transferr(ing|ed) logs to target controller \\([234] sent\\)"},
	})
}

func (s *Suite) TestLogTransferChecksLatestTime(c *tc.C) {
	s.modelMigrationService.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	t := time.Date(2016, 12, 2, 10, 39, 10, 20, time.UTC)
	s.connection.latestLogTime = t

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]testhelpers.StubCall{
			{FuncName: "controllerConfigService.ControllerConfig", Args: nil},
			getLokiConfigCall,
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []any{t}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "modelMigrationService.SetMigrationPhase", Args: []any{coremigration.REAP}},
			{FuncName: "modelMigrationService.MarkModelAsGone", Args: nil},
		},
	))
}

func safeSend(c *tc.C, d chan<- common.LogMessage, message common.LogMessage) {
	select {
	case d <- message:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out sending log message")
	}
}

func (s *Suite) checkWorkerReturns(c *tc.C, expected error) {
	err := s.runWorker(c)
	c.Check(errors.Cause(err), tc.Equals, expected)
}

func (s *Suite) checkWorkerErr(c *tc.C, expected string) {
	err := s.runWorker(c)
	c.Check(err, tc.ErrorMatches, expected)
}

func (s *Suite) runWorker(c *tc.C) error {
	w, err := migrationmaster.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	return workertest.CheckKilled(c, w)
}

func (s *Suite) waitForStubCalls(c *tc.C, expectedCallNames []string) {
	var callNames []string
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		callNames = stubCallNames(s.stub)
		if reflect.DeepEqual(callNames, expectedCallNames) {
			return
		}
	}
	c.Fatalf("failed to see expected calls\nobtained: %v\nexpected: %v",
		callNames, expectedCallNames)
}

func (s *Suite) checkMinionWaitWatchError(c *tc.C, phase coremigration.Phase) {
	s.modelMigrationService.minionReportsWatchErr = errors.New("boom")
	s.modelMigrationService.queueStatus(s.makeStatus(phase))

	s.checkWorkerErr(c, "boom")
}

func (s *Suite) checkMinionWaitGetError(c *tc.C, phase coremigration.Phase) {
	s.modelMigrationService.queueStatus(s.makeStatus(phase))

	s.modelMigrationService.minionReportsErr = errors.New("boom")
	s.modelMigrationService.triggerMinionReports()

	s.checkWorkerErr(c, "boom")
}

// assertExpectedCallArgs checks that the stub has been called with the
// expected arguments. It ignores the facade versions map on the Prechecks
// call because that's an implementation detail of the api facade, not the
// worker. As long as it's non-zero, otherwise we don't care.
func assertExpectedCallArgs(c *tc.C, stub *testhelpers.Stub, expectedCalls []testhelpers.StubCall) {
	stub.CheckCallNames(c, callNames(expectedCalls)...)
	for i, call := range expectedCalls {
		stubCall := stub.Calls()[i]

		if call.FuncName == "MigrationTarget.Prechecks" {
			mc := tc.NewMultiChecker()
			mc.AddExpr("_[0].FacadeVersions", tc.Not(tc.HasLen), 0)
			c.Assert(stubCall.Args, mc, call.Args, tc.Commentf("call %s", call.FuncName))
			continue
		}

		if call.FuncName == "UploadBinaries" {
			mc := tc.NewMultiChecker()
			mc.AddExpr(`_[5]`, tc.NotNil)
			c.Assert(stubCall.Args[:5], mc, call.Args[:5], tc.Commentf("call %s", call.FuncName))
			continue
		}

		c.Assert(stubCall, tc.DeepEquals, call, tc.Commentf("call %s", call.FuncName))
	}
}

func stubCallNames(stub *testhelpers.Stub) []string {
	var out []string
	for _, call := range stub.Calls() {
		out = append(out, call.FuncName)
	}
	return out
}

func newStubGuard(stub *testhelpers.Stub) *stubGuard {
	return &stubGuard{stub: stub}
}

type stubGuard struct {
	stub        *testhelpers.Stub
	unlockErr   error
	lockdownErr error
}

func (g *stubGuard) Lockdown(ctx context.Context) error {
	g.stub.AddCall("guard.Lockdown")
	return g.lockdownErr
}

func (g *stubGuard) Unlock(ctx context.Context) error {
	g.stub.AddCall("guard.Unlock")
	return g.unlockErr
}

func newStubMasterFacade(stub *testhelpers.Stub) *stubMasterFacade {
	return &stubMasterFacade{
		stub: stub,
	}
}

type stubMasterFacade struct {
	stub *testhelpers.Stub

	prechecksErr error

	logMessages func(chan<- common.LogMessage)
	streamErr   error
}

func newStubModelMigrationService(stub *testhelpers.Stub) *stubModelMigrationService {
	return &stubModelMigrationService{
		stub:           stub,
		watcherChanges: make(chan struct{}, 999),
		// Give minionReportsChanges a larger-than-required buffer to
		// support waits at a number of phases.
		minionReportsChanges: make(chan struct{}, 999),
	}
}

type stubModelMigrationService struct {
	stub *testhelpers.Stub

	watcherChanges chan struct{}
	watchErr       error
	status         []coremigration.MigrationStatus
	statusErr      error

	minionReportsChanges  chan struct{}
	minionReportsWatchErr error
	minionReports         []coremigration.MinionReports
	minionReportsErr      error

	statuses []string
}

func (s *stubModelMigrationService) triggerWatcher() {
	select {
	case s.watcherChanges <- struct{}{}:
	default:
		panic("migration watcher channel unexpectedly closed")
	}
}

func (s *stubModelMigrationService) queueStatus(status coremigration.MigrationStatus) {
	s.status = append(s.status, status)
	s.triggerWatcher()
}

func (s *stubModelMigrationService) WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error) {
	s.stub.AddCall("modelMigrationService.WatchForMigration")
	if s.watchErr != nil {
		return nil, s.watchErr
	}
	return newMockWatcher(s.watcherChanges), nil
}

func (s *stubModelMigrationService) Migration(ctx context.Context) (modelmigration.Migration, error) {
	s.stub.AddCall("modelMigrationService.Migration")
	if s.statusErr != nil {
		return modelmigration.Migration{}, s.statusErr
	}
	if len(s.status) == 0 {
		panic("no status queued to report")
	}
	out := s.status[0]
	s.status = s.status[1:]
	return modelmigration.Migration{
		UUID:             out.MigrationId,
		Phase:            out.Phase,
		PhaseChangedTime: out.PhaseChangedTime,
		Target:           out.TargetInfo,
	}, nil
}

func (s *stubModelMigrationService) SetMigrationPhase(ctx context.Context, phase coremigration.Phase) error {
	s.stub.AddCall("modelMigrationService.SetMigrationPhase", phase)
	return nil
}

func (s *stubModelMigrationService) SetMigrationStatusMessage(ctx context.Context, message string) error {
	s.statuses = append(s.statuses, message)
	return nil
}

func (s *stubModelMigrationService) MarkModelAsGone(ctx context.Context) error {
	s.stub.AddCall("modelMigrationService.MarkModelAsGone")
	return nil
}

func (s *stubModelMigrationService) triggerMinionReports() {
	select {
	case s.minionReportsChanges <- struct{}{}:
	default:
		panic("minion reports watcher channel unexpectedly closed")
	}
}

func (s *stubModelMigrationService) queueMinionReports(r coremigration.MinionReports) {
	s.minionReports = append(s.minionReports, r)
	s.triggerMinionReports()
}

func (s *stubModelMigrationService) WatchMinionReports(ctx context.Context) (watcher.NotifyWatcher, error) {
	s.stub.AddCall("modelMigrationService.WatchMinionReports")
	if s.minionReportsWatchErr != nil {
		return nil, s.minionReportsWatchErr
	}
	return newMockWatcher(s.minionReportsChanges), nil
}

func (s *stubModelMigrationService) MinionReports(ctx context.Context) (coremigration.MinionReports, error) {
	s.stub.AddCall("modelMigrationService.MinionReports")
	if s.minionReportsErr != nil {
		return coremigration.MinionReports{}, s.minionReportsErr
	}
	if len(s.minionReports) == 0 {
		return coremigration.MinionReports{}, errors.NotFoundf("reports")
	}
	r := s.minionReports[0]
	s.minionReports = s.minionReports[1:]
	return r, nil
}

func (s *stubModelMigrationService) SourceControllerInfo(ctx context.Context) (coremigration.SourceControllerInfo, error) {
	s.stub.AddCall("modelMigrationService.SourceControllerInfo")
	return coremigration.SourceControllerInfo{
		ControllerTag:   sourceControllerTag,
		ControllerAlias: "mycontroller",
		Addrs:           []string{"source-addr"},
		CACert:          "cacert",
	}, nil
}

func (f *stubMasterFacade) Prechecks(ctx context.Context) error {
	f.stub.AddCall("facade.Prechecks")
	return f.prechecksErr
}

type stubExportService struct {
	stub                   *testhelpers.Stub
	exportErr              error
	controllerModelInfoErr error
}

func (s *stubExportService) Export(ctx context.Context) (*domainexport.ModelExport, error) {
	s.stub.AddCall("exportService.Export")
	if s.exportErr != nil {
		return nil, s.exportErr
	}
	return &domainexport.ModelExport{
		Version: fakeExportVersion,
		Payload: fakeExportPayload,
	}, nil
}

func (s *stubExportService) GetControllerModelInfo(ctx context.Context) (coremodelmigration.ControllerModelInfo, error) {
	s.stub.AddCall("exportService.GetControllerModelInfo")
	if s.controllerModelInfoErr != nil {
		return coremodelmigration.ControllerModelInfo{}, s.controllerModelInfoErr
	}
	return fakeControllerModelInfo, nil
}

type stubControllerConfigService struct {
	stub *testhelpers.Stub
}

func (s *stubControllerConfigService) ControllerConfig(ctx context.Context) (controller.Config, error) {
	s.stub.AddCall("controllerConfigService.ControllerConfig")
	// An empty config falls back to defaults, including the 15 minute
	// migration minion wait maximum the tests rely on.
	return controller.Config{}, nil
}

type stubModelAgentService struct {
	stub *testhelpers.Stub
}

func (s *stubModelAgentService) GetModelAgentBinaryMetadata(
	ctx context.Context,
) (map[machine.Name]coreagentbinary.Metadata, map[unit.Name]coreagentbinary.Metadata, error) {
	s.stub.AddCall("modelAgentService.GetModelAgentBinaryMetadata")
	return fakeMachineTools, nil, nil
}

type stubResourceService struct {
	stub      *testhelpers.Stub
	resources []coreresource.Resource
}

func (s *stubResourceService) ListAllModelResources(ctx context.Context) ([]coreresource.Resource, error) {
	s.stub.AddCall("resourceService.ListAllModelResources")
	return s.resources, nil
}

func (s *stubResourceService) GetResourceUUIDByApplicationAndResourceName(ctx context.Context, appName, resName string) (coreresource.UUID, error) {
	s.stub.AddCall("resourceService.GetResourceUUIDByApplicationAndResourceName", appName, resName)
	return coreresource.UUID(""), nil
}

func (s *stubResourceService) OpenResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, io.ReadCloser, error) {
	s.stub.AddCall("resourceService.OpenResource", resourceUUID)
	return coreresource.Resource{}, io.NopCloser(strings.NewReader("")), nil
}

type stubCharmService struct {
	migrationmaster.CharmService

	stub *testhelpers.Stub
}

func (s *stubCharmService) ListCharmLocators(ctx context.Context, names ...string) ([]applicationcharm.CharmLocator, error) {
	s.stub.AddCall("charmService.ListCharmLocators")
	return fakeCharmLocators, nil
}

type stubLoggingService struct {
	migrationmaster.LoggingService

	stub *testhelpers.Stub

	// lokiEnabled is returned by IsLokiEnabled.
	lokiEnabled bool
}

func (s *stubLoggingService) IsLokiEnabled(ctx context.Context) (bool, error) {
	s.stub.AddCall("loggingService.IsLokiEnabled")
	return s.lokiEnabled, nil
}

func (f *stubMasterFacade) StreamModelLog(_ context.Context, start time.Time) (<-chan common.LogMessage, error) {
	f.stub.AddCall("StreamModelLog", start)
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	result := make(chan common.LogMessage)
	messageFunc := f.logMessages
	if messageFunc == nil {
		messageFunc = func(chan<- common.LogMessage) {}
	}
	go func() {
		defer close(result)
		messageFunc(result)
	}()
	return result, nil
}

func newMockWatcher(changes chan struct{}) *mockWatcher {
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

func (w *mockWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

type stubConnection struct {
	c *tc.C
	api.Connection
	stub          *testhelpers.Stub
	prechecksErr  error
	importErr     error
	controllerTag names.ControllerTag

	streamErr error
	logStream *mockStream

	latestLogErr  error
	latestLogTime time.Time

	machineErrs     []string
	checkMachineErr error

	facadeVersion int

	controllerVersion params.ControllerVersionResults
}

func (c *stubConnection) BestFacadeVersion(string) int {
	return c.facadeVersion
}

func (c *stubConnection) APICall(ctx context.Context, objType string, _ int, _, request string, args, response any) error {
	c.stub.AddCall(objType+"."+request, args)

	if objType == "MigrationTarget" {
		switch request {
		case "Prechecks":
			return c.prechecksErr
		case "Import":
			return c.importErr
		case "Activate", "AdoptResources":
			return nil
		case "LatestLogTime":
			responseTime := response.(*time.Time)
			// This is needed because even if a zero time comes back
			// from the API it will have a timezone attached.
			*responseTime = c.latestLogTime.In(time.UTC)
			return c.latestLogErr
		case "CheckMachines":
			results := response.(*params.ErrorResults)
			for _, msg := range c.machineErrs {
				results.Results = append(results.Results, params.ErrorResult{
					Error: apiservererrors.ServerError(errors.New(msg)),
				})
			}
			return c.checkMachineErr
		}
	} else if objType == "Controller" {
		switch request {
		case "ControllerVersion":
			c.c.Logf("objType %q request %q, args %#v", objType, request, args)
			controllerVersion := response.(*params.ControllerVersionResults)
			*controllerVersion = c.controllerVersion
			return nil
		}
	}
	return errors.New("unexpected API call")
}

func (c *stubConnection) Client() *apiclient.Client {
	// This is kinda crappy but the *Client doesn't have to be
	// functional...
	return new(apiclient.Client)
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}

func (c *stubConnection) ControllerTag() names.ControllerTag {
	return c.controllerTag
}

func (c *stubConnection) ConnectControllerStream(_ context.Context, path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	c.stub.AddCall("ConnectControllerStream", path, attrs, headers)
	if c.streamErr != nil {
		return nil, c.streamErr
	}
	return c.logStream, nil
}

func makeStubUploadBinaries(stub *testhelpers.Stub) func(context.Context, migration.UploadBinariesConfig, logger.Logger) error {
	return func(_ context.Context, config migration.UploadBinariesConfig, _ logger.Logger) error {
		stub.AddCall(
			"UploadBinaries",
			config.Charms,
			config.CharmService,
			config.Tools,
			config.AgentBinaryStore,
			config.Resources,
			config.ResourceDownloader,
		)
		return nil
	}
}

// nullUploadBinaries is a UploadBinaries variant which is intended to
// not get called.
func nullUploadBinaries(context.Context, migration.UploadBinariesConfig, logger.Logger) error {
	panic("should not get called")
}

var fakeAgentBinaryStore = struct{ migration.AgentBinaryStore }{}

func joinCalls(allCalls ...[]testhelpers.StubCall) (out []testhelpers.StubCall) {
	for _, calls := range allCalls {
		out = append(out, calls...)
	}
	return
}

func callNames(calls []testhelpers.StubCall) []string {
	var out []string
	for _, call := range calls {
		out = append(out, call.FuncName)
	}
	return out
}

func makeMinionReports(p coremigration.Phase) coremigration.MinionReports {
	return coremigration.MinionReports{
		MigrationId:  "model-uuid:2",
		Phase:        p,
		TotalCount:   5,
		SuccessCount: 5,
		UnknownCount: 0,
	}
}

type mockStream struct {
	base.Stream
	c          *tc.C
	written    []params.LogRecord
	writeErr   error
	closeCount int
}

func (s *mockStream) WriteJSON(v any) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	rec, ok := v.(params.LogRecord)
	if !ok {
		s.c.Errorf("unexpected value written to stream: %v", v)
		return nil
	}
	s.written = append(s.written, rec)
	return nil
}

func (s *mockStream) Close() error {
	s.closeCount++
	return nil
}
