// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"context"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremigration "github.com/juju/juju/core/migration"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type Suite struct {
	coretesting.BaseSuite
	clock         *testclock.Clock
	stub          *jujutesting.Stub
	connection    *stubConnection
	connectionErr error
	facade        *stubMasterFacade
	config        migrationmaster.Config
}

var _ = gc.Suite(&Suite{})

var (
	fakeModelBytes      = []byte("model")
	sourceControllerTag = names.NewControllerTag("source-controller-uuid")
	targetControllerTag = names.NewControllerTag("controller-uuid")
	modelUUID           = "model-uuid"
	modelTag            = names.NewModelTag(modelUUID)
	modelName           = "model-name"
	ownerTag            = names.NewUserTag("owner")
	modelVersion        = version.MustParse("1.2.4")

	// Define stub calls that commonly appear in tests here to allow reuse.
	apiOpenControllerCall = jujutesting.StubCall{
		FuncName: "apiOpen",
		Args: []interface{}{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
			},
			migration.ControllerDialOpts(nil),
		},
	}
	importCall = jujutesting.StubCall{
		FuncName: "MigrationTarget.Import",
		Args: []interface{}{
			params.SerializedModel{Bytes: fakeModelBytes},
		},
	}
	activateCall = jujutesting.StubCall{
		FuncName: "MigrationTarget.Activate",
		Args: []interface{}{
			params.ActivateModelArgs{
				ModelTag:        modelTag.String(),
				ControllerTag:   sourceControllerTag.String(),
				ControllerAlias: "mycontroller",
				SourceAPIAddrs:  []string{"source-addr"},
				SourceCACert:    "cacert",
				CrossModelUUIDs: []string{"related-model-uuid"},
			},
		},
	}
	checkMachinesCall = jujutesting.StubCall{
		FuncName: "MigrationTarget.CheckMachines",
		Args: []interface{}{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	adoptResourcesCall = jujutesting.StubCall{
		FuncName: "MigrationTarget.AdoptResources",
		Args: []interface{}{
			params.AdoptResourcesArgs{
				ModelTag:                modelTag.String(),
				SourceControllerVersion: jujuversion.Current,
			},
		},
	}
	latestLogTimeCall = jujutesting.StubCall{
		FuncName: "MigrationTarget.LatestLogTime",
		Args: []interface{}{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	apiCloseCall = jujutesting.StubCall{FuncName: "Connection.Close"}
	abortCall    = jujutesting.StubCall{
		FuncName: "MigrationTarget.Abort",
		Args: []interface{}{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	watchStatusLockdownCalls = []jujutesting.StubCall{
		{FuncName: "facade.Watch", Args: nil},
		{FuncName: "facade.MigrationStatus", Args: nil},
		{FuncName: "guard.Lockdown", Args: nil},
	}
	prechecksCalls = []jujutesting.StubCall{
		{FuncName: "facade.ModelInfo", Args: nil},
		{FuncName: "facade.Prechecks", Args: []interface{}{}},
		apiOpenControllerCall,
		{FuncName: "MigrationTarget.Prechecks", Args: []interface{}{params.MigrationModelInfo{
			UUID:         modelUUID,
			Name:         modelName,
			OwnerTag:     ownerTag.String(),
			AgentVersion: modelVersion,
			ModelDescription: func() []byte {
				modelDescription := description.NewModel(description.ModelArgs{})
				bytes, err := description.Serialize(modelDescription)
				if err != nil {
					panic(err)
				}
				return bytes
			}(),
		}}},
		apiCloseCall,
	}
	abortCalls = []jujutesting.StubCall{
		{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORT}},
		apiOpenControllerCall,
		abortCall,
		apiCloseCall,
		{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORTDONE}},
	}
	openDestLogStreamCall = jujutesting.StubCall{FuncName: "ConnectControllerStream", Args: []interface{}{
		"/migrate/logtransfer",
		url.Values{},
		http.Header{
			textproto.CanonicalMIMEHeaderKey(params.MigrationModelHTTPHeader): {modelUUID},
		},
	}}
)

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Now())
	s.stub = new(jujutesting.Stub)
	s.connection = &stubConnection{
		c:             c,
		stub:          s.stub,
		controllerTag: targetControllerTag,
		logStream:     &mockStream{},
		controllerVersion: params.ControllerVersionResults{
			Version: "2.9.99",
		},
		facadeVersion: 2,
	}
	s.connectionErr = nil

	s.facade = newStubMasterFacade(s.stub)

	// The default worker Config used by most of the tests. Tests may
	// tweak parts of this as needed.
	s.config = migrationmaster.Config{
		ModelUUID:       utils.MustNewUUID().String(),
		Facade:          s.facade,
		Guard:           newStubGuard(s.stub),
		APIOpen:         s.apiOpen,
		UploadBinaries:  nullUploadBinaries,
		CharmDownloader: fakeCharmDownloader,
		ToolsDownloader: fakeToolsDownloader,
		Clock:           s.clock,
	}
}

func (s *Suite) apiOpen(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
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
			ControllerTag: targetControllerTag,
			Addrs:         []string{"1.2.3.4:5"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("admin"),
			Password:      "secret",
		},
	}
}

func (s *Suite) TestSuccessfulMigration(c *gc.C) {
	s.facade.exportedResources = []coremigration.SerializedModelResource{{
		ApplicationRevision: resourcetesting.NewResource(c, nil, "blob", "app", "").Resource,
	}}

	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.SUCCESS))
	s.config.UploadBinaries = makeStubUploadBinaries(s.stub)

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	assertExpectedCallArgs(c, s.stub, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
		},

		// QUIESCE
		prechecksCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
		},
		prechecksCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.IMPORT}},

			//IMPORT
			{FuncName: "facade.Export", Args: nil},
			apiOpenControllerCall,
			importCall,
			{FuncName: "UploadBinaries", Args: []interface{}{
				[]string{"charm0", "charm1"},
				fakeCharmDownloader,
				map[version.Binary]string{
					version.MustParseBinary("2.1.0-ubuntu-amd64"): "/tools/0",
				},
				fakeToolsDownloader,
				s.facade.exportedResources,
				s.facade,
			}},
			apiCloseCall, // for target controller
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.PROCESSRELATIONS}},

			// PROCESSRELATIONS
			{FuncName: "facade.ProcessRelations", Args: []interface{}{""}},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.VALIDATION}},

			// VALIDATION
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			{FuncName: "facade.SourceControllerInfo", Args: nil},
			activateCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.SUCCESS}},

			// SUCCESS
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.LOGTRANSFER}},

			// LOGTRANSFER
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},

			// REAP
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		}),
	)
}

func (s *Suite) TestIncompatibleTarget(c *gc.C) {
	s.connection.facadeVersion = 1
	s.facade.exportedResources = []coremigration.SerializedModelResource{{
		ApplicationRevision: resourcetesting.NewResource(c, nil, "blob", "app", "").Resource,
	}}

	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.SUCCESS))
	s.config.UploadBinaries = makeStubUploadBinaries(s.stub)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
		},

		// QUIESCE
		[]jujutesting.StubCall{
			{FuncName: "facade.ModelInfo", Args: nil},
			{FuncName: "facade.Prechecks", Args: []interface{}{}},
			apiOpenControllerCall,
			{FuncName: "facade.SourceControllerInfo", Args: nil},
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.facade.queueMinionReports(makeMinionReports(coremigration.SUCCESS))

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.LOGTRANSFER}},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.ABORTDONE))

	w, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"facade.Watch",
		"facade.MigrationStatus",
		"guard.Unlock",
	})
}

func (s *Suite) TestPreviouslyCompletedMigration(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.DONE))
	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "facade.Watch", Args: nil},
		{FuncName: "facade.MigrationStatus", Args: nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	s.facade.watchErr = errors.New("boom")
	s.checkWorkerErr(c, "watching for migration: boom")
}

func (s *Suite) TestStatusError(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.statusErr = errors.New("splat")

	s.checkWorkerErr(c, "retrieving migration status: splat")
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "facade.Watch", Args: nil},
		{FuncName: "facade.MigrationStatus", Args: nil},
	})
}

func (s *Suite) TestStatusNotFound(c *gc.C) {
	s.facade.statusErr = &params.Error{Code: params.CodeNotFound}
	s.facade.triggerWatcher()

	w, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"facade.Watch",
		"facade.MigrationStatus",
		"guard.Unlock",
	})
}

func (s *Suite) TestUnlockError(c *gc.C) {
	s.facade.statusErr = &params.Error{Code: params.CodeNotFound}
	s.facade.triggerWatcher()
	guard := newStubGuard(s.stub)
	guard.unlockErr = errors.New("pow")
	s.config.Guard = guard

	s.checkWorkerErr(c, "pow")
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "facade.Watch", Args: nil},
		{FuncName: "facade.MigrationStatus", Args: nil},
		{FuncName: "guard.Unlock", Args: nil},
	})
}

func (s *Suite) TestLockdownError(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	guard := newStubGuard(s.stub)
	guard.lockdownErr = errors.New("biff")
	s.config.Guard = guard

	s.checkWorkerErr(c, "biff")
	s.stub.CheckCalls(c, watchStatusLockdownCalls)
}

func (s *Suite) TestQUIESCEMinionWaitWatchError(c *gc.C) {
	s.checkMinionWaitWatchError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEMinionWaitGetError(c *gc.C) {
	s.checkMinionWaitGetError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEFailedAgent(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.QUIESCE,
		FailedMachines: []string{"42"}, // a machine failed
	})

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)

	expectedCalls := joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
		},
		prechecksCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
		},
		abortCalls,
	)

	assertExpectedCallArgs(c, s.stub, expectedCalls)
}

func (s *Suite) TestQUIESCEWrongController(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.controllerTag = names.NewControllerTag("another-controller")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.ModelInfo", Args: nil},
			{FuncName: "facade.Prechecks", Args: []interface{}{}},
			apiOpenControllerCall,
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCESourceChecksFail(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.prechecksErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.ModelInfo", Args: nil},
			{FuncName: "facade.Prechecks", Args: []interface{}{}},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCEModelInfoFail(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.modelInfoErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.ModelInfo", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCETargetChecksFail(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.prechecksErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	assertExpectedCallArgs(c, s.stub, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
		},
		prechecksCalls,
		abortCalls,
	))
}

func (s *Suite) TestProcessRelationsFailure(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.PROCESSRELATIONS))
	s.facade.processRelationsErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.ProcessRelations", Args: []interface{}{""}},
		},
		abortCalls,
	))
}

func (s *Suite) TestExportFailure(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.facade.exportErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.Export", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestAPIOpenFailure(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connectionErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.Export", Args: nil},
			apiOpenControllerCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORT}},
			apiOpenControllerCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestImportFailure(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.IMPORT))
	s.connection.importErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.Export", Args: nil},
			apiOpenControllerCall,
			importCall,
			apiCloseCall,
		},
		abortCalls,
	))
}

func (s *Suite) TestVALIDATIONMinionWaitWatchError(c *gc.C) {
	s.checkMinionWaitWatchError(c, coremigration.VALIDATION)
}

func (s *Suite) TestVALIDATIONMinionWaitGetError(c *gc.C) {
	s.checkMinionWaitGetError(c, coremigration.VALIDATION)
}

func (s *Suite) TestVALIDATIONFailedAgent(c *gc.C) {
	// Set the last phase change status to be further back
	// in time than the max wait time for minion reports.
	sts := s.makeStatus(coremigration.VALIDATION)
	sts.PhaseChangedTime = time.Now().Add(-20 * time.Minute)
	s.facade.queueStatus(sts)

	w, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// Queue the reports *after* the watcher is started.
	// The test will only pass if the minion wait timeout
	// is independent of the phase change time.
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.VALIDATION,
		FailedMachines: []string{"42"}, // a machine failed
	})

	err = workertest.CheckKilled(c, w)
	c.Check(errors.Cause(err), gc.Equals, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestVALIDATIONCheckMachinesOneError(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))

	s.connection.machineErrs = []string{"been so strange"}
	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
		abortCalls,
	))
	lastMessages := s.facade.statuses[len(s.facade.statuses)-2:]
	c.Assert(lastMessages, gc.DeepEquals, []string{
		"machine sanity check failed, 1 error found",
		"aborted, removing model from target controller: machine sanity check failed, 1 error found",
	})
}

func (s *Suite) TestVALIDATIONCheckMachinesSeveralErrors(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.connection.machineErrs = []string{"been so strange", "lit up"}
	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
		abortCalls,
	))
	lastMessages := s.facade.statuses[len(s.facade.statuses)-2:]
	c.Assert(lastMessages, gc.DeepEquals, []string{
		"machine sanity check failed, 2 errors found",
		"aborted, removing model from target controller: machine sanity check failed, 2 errors found",
	})
}

func (s *Suite) TestVALIDATIONCheckMachinesOtherError(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.connection.checkMachineErr = errors.Errorf("something went bang")

	s.checkWorkerReturns(c, s.connection.checkMachineErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			checkMachinesCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitWatchError(c *gc.C) {
	s.checkMinionWaitWatchError(c, coremigration.SUCCESS)
}

func (s *Suite) TestSUCCESSMinionWaitGetError(c *gc.C) {
	s.checkMinionWaitGetError(c, coremigration.SUCCESS)
}

func (s *Suite) TestSUCCESSMinionWaitFailedMachine(c *gc.C) {
	// With the SUCCESS phase the master should wait for all reports,
	// continuing even if some minions report failure.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.SUCCESS,
		FailedMachines: []string{"42"},
	})

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.LOGTRANSFER}},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitFailedUnit(c *gc.C) {
	// See note for TestMinionWaitFailedMachine above.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId:        "model-uuid:2",
		Phase:              coremigration.SUCCESS,
		FailedUnits:        []string{"foo/2"},
		FailedApplications: []string{"bar"},
	})

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			{FuncName: "facade.MinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.LOGTRANSFER}},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitTimeout(c *gc.C) {
	// The SUCCESS phase is special in that even if some minions fail
	// to report the migration should continue. There's no turning
	// back from SUCCESS.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))

	w, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for clock.After call")
	}

	// Move time ahead in order to trigger timeout.
	s.clock.Advance(15 * time.Minute)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, migrationmaster.ErrMigrated)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{FuncName: "facade.WatchMinionReports", Args: nil},
			apiOpenControllerCall,
			adoptResourcesCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.LOGTRANSFER}},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestMinionWaitWrongPhase(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))

	// Have the phase in the minion reports be different from the
	// migration status. This shouldn't happen but the migrationmaster
	// should handle it.
	s.facade.queueMinionReports(makeMinionReports(coremigration.IMPORT))

	s.checkWorkerErr(c,
		`minion reports phase \(IMPORT\) does not match migration phase \(SUCCESS\)`)
}

func (s *Suite) TestMinionWaitMigrationIdChanged(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))

	// Have the migration id in the minion reports be different from
	// the migration status. This shouldn't happen but the
	// migrationmaster should handle it.
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId: "blah",
		Phase:       coremigration.SUCCESS,
	})

	s.checkWorkerErr(c,
		"unexpected migration id in minion reports, got blah, expected model-uuid:2")
}

func (s *Suite) assertAPIConnectWithMacaroon(c *gc.C, authUser names.UserTag) {
	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	status := s.makeStatus(coremigration.ABORT)
	status.TargetInfo.AuthTag = authUser

	// Set up macaroon based auth to the target.
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macs := []macaroon.Slice{{mac}}
	status.TargetInfo.Password = ""
	status.TargetInfo.Macaroons = macs

	s.facade.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	var apiUser names.Tag
	if authUser.IsLocal() {
		apiUser = authUser
	}
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{
				FuncName: "apiOpen",
				Args: []interface{}{
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
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestAPIConnectWithMacaroonLocalUser(c *gc.C) {
	s.assertAPIConnectWithMacaroon(c, names.NewUserTag("admin"))
}

func (s *Suite) TestAPIConnectWithMacaroonExternalUser(c *gc.C) {
	s.assertAPIConnectWithMacaroon(c, names.NewUserTag("fred@external"))
}

func (s *Suite) TestAPIConnectionWithToken(c *gc.C) {
	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	status := s.makeStatus(coremigration.ABORT)
	authUser := names.NewUserTag("fred@external")
	status.TargetInfo.AuthTag = authUser

	// Set up token based auth to the target.
	status.TargetInfo.Password = ""
	status.TargetInfo.Macaroons = nil
	status.TargetInfo.Token = "token"

	s.facade.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	expectedLoginProvider := api.NewSessionTokenLoginProvider("token", nil, nil)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			{
				FuncName: "apiOpen",
				Args: []interface{}{
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
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningTargetAPI(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connectionErr = errors.New("people of earth")

	s.checkWorkerReturns(c, s.connectionErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorGettingStartTime(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connection.latestLogErr = errors.New("tender vittles")

	s.checkWorkerReturns(c, s.connection.latestLogErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningLogSource(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.facade.streamErr = errors.New("chicken bones")

	s.checkWorkerReturns(c, s.facade.streamErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorOpeningLogDest(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.connection.streamErr = errors.New("tule lake shuffle")

	s.checkWorkerReturns(c, s.connection.streamErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
		},
	))
}

func (s *Suite) TestLogTransferErrorWriting(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	s.facade.logMessages = func(d chan<- common.LogMessage) {
		safeSend(c, d, common.LogMessage{Message: "the go team"})
	}
	s.connection.logStream.writeErr = errors.New("bottle rocket")
	s.checkWorkerReturns(c, s.connection.logStream.writeErr)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
		},
	))
	c.Assert(s.connection.logStream.closeCount, gc.Equals, 1)
}

func (s *Suite) TestLogTransferSendsRecords(c *gc.C) {
	t1, err := time.Parse("2006-01-02 15:04", "2016-11-28 16:11")
	c.Assert(err, jc.ErrorIsNil)
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
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
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{time.Time{}}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
	c.Assert(s.connection.logStream.written, gc.DeepEquals, []params.LogRecord{
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
	c.Assert(s.connection.logStream.closeCount, gc.Equals, 1)
}

func (s *Suite) TestLogTransferReportsProgress(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	messages := []common.LogMessage{
		{Message: "captain beefheart"},
		{Message: "super furry animals"},
		{Message: "ezra furman"},
		{Message: "these new puritans"},
	}
	s.facade.logMessages = func(d chan<- common.LogMessage) {
		for _, message := range messages {
			safeSend(c, d, message)
			c.Assert(s.clock.WaitAdvance(20*time.Second, coretesting.LongWait, 1), jc.ErrorIsNil)
		}
	}

	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("migrationmaster-tests", &logWriter), jc.ErrorIsNil)
	defer func() {
		_, _ = loggo.RemoveWriter("migrationmaster-tests")
		logWriter.Clear()
	}()

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)

	c.Assert(logWriter.Log()[:3], jc.LogMatches, []string{
		"successful, transferring logs to target controller \\(0 sent\\)",
		// This is a bit of a punt, but without accepting a range
		// we sometimes see this test failing on loaded test machines.
		"successful, transferring logs to target controller \\([23] sent\\)",
		"successful, transferr(ing|ed) logs to target controller \\([234] sent\\)",
	})
}

func (s *Suite) TestLogTransferChecksLatestTime(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.LOGTRANSFER))
	t := time.Date(2016, 12, 2, 10, 39, 10, 20, time.UTC)
	s.connection.latestLogTime = t

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{FuncName: "facade.MinionReportTimeout", Args: nil},
			apiOpenControllerCall,
			latestLogTimeCall,
			{FuncName: "StreamModelLog", Args: []interface{}{t}},
			openDestLogStreamCall,
			apiCloseCall,
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.REAP}},
			{FuncName: "facade.Reap", Args: nil},
			{FuncName: "facade.SetPhase", Args: []interface{}{coremigration.DONE}},
		},
	))
}

func safeSend(c *gc.C, d chan<- common.LogMessage, message common.LogMessage) {
	select {
	case d <- message:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out sending log message")
	}
}

func (s *Suite) checkWorkerReturns(c *gc.C, expected error) {
	err := s.runWorker(c)
	c.Check(errors.Cause(err), gc.Equals, expected)
}

func (s *Suite) checkWorkerErr(c *gc.C, expected string) {
	err := s.runWorker(c)
	c.Check(err, gc.ErrorMatches, expected)
}

func (s *Suite) runWorker(c *gc.C) error {
	w, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	return workertest.CheckKilled(c, w)
}

func (s *Suite) waitForStubCalls(c *gc.C, expectedCallNames []string) {
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

func (s *Suite) checkMinionWaitWatchError(c *gc.C, phase coremigration.Phase) {
	s.facade.minionReportsWatchErr = errors.New("boom")
	s.facade.queueStatus(s.makeStatus(phase))

	s.checkWorkerErr(c, "boom")
}

func (s *Suite) checkMinionWaitGetError(c *gc.C, phase coremigration.Phase) {
	s.facade.queueStatus(s.makeStatus(phase))

	s.facade.minionReportsErr = errors.New("boom")
	s.facade.triggerMinionReports()

	s.checkWorkerErr(c, "boom")
}

// assertExpectedCallArgs checks that the stub has been called with the
// expected arguments. It ignores the facade versions map on the Prechecks
// call because that's an implementation detail of the api facade, not the
// worker. As long as it's non-zero, otherwise we don't care.
func assertExpectedCallArgs(c *gc.C, stub *jujutesting.Stub, expectedCalls []jujutesting.StubCall) {
	stub.CheckCallNames(c, callNames(expectedCalls)...)
	for i, call := range expectedCalls {
		stubCall := stub.Calls()[i]

		if call.FuncName == "MigrationTarget.Prechecks" {
			mc := jc.NewMultiChecker()
			mc.AddExpr("_.FacadeVersions", gc.Not(gc.HasLen), 0)

			c.Assert(stubCall.Args, mc, call.Args, gc.Commentf("call %s", call.FuncName))
			continue
		}

		c.Assert(stubCall, jc.DeepEquals, call, gc.Commentf("call %s", call.FuncName))
	}
}

func stubCallNames(stub *jujutesting.Stub) []string {
	var out []string
	for _, call := range stub.Calls() {
		out = append(out, call.FuncName)
	}
	return out
}

func newStubGuard(stub *jujutesting.Stub) *stubGuard {
	return &stubGuard{stub: stub}
}

type stubGuard struct {
	stub        *jujutesting.Stub
	unlockErr   error
	lockdownErr error
}

func (g *stubGuard) Lockdown(fortress.Abort) error {
	g.stub.AddCall("guard.Lockdown")
	return g.lockdownErr
}

func (g *stubGuard) Unlock() error {
	g.stub.AddCall("guard.Unlock")
	return g.unlockErr
}

func newStubMasterFacade(stub *jujutesting.Stub) *stubMasterFacade {
	return &stubMasterFacade{
		stub:           stub,
		watcherChanges: make(chan struct{}, 999),

		// Give minionReportsChanges a larger-than-required buffer to
		// support waits at a number of phases.
		minionReportsChanges: make(chan struct{}, 999),
		minionReportTimeout:  15 * time.Minute,
	}
}

type stubMasterFacade struct {
	migrationmaster.Facade

	stub *jujutesting.Stub

	watcherChanges chan struct{}
	watchErr       error
	status         []coremigration.MigrationStatus
	statusErr      error

	prechecksErr        error
	modelInfoErr        error
	exportErr           error
	processRelationsErr error

	logMessages func(chan<- common.LogMessage)
	streamErr   error

	minionReportsChanges  chan struct{}
	minionReportsWatchErr error
	minionReports         []coremigration.MinionReports
	minionReportsErr      error
	minionReportTimeout   time.Duration

	exportedResources []coremigration.SerializedModelResource

	statuses []string
}

func (f *stubMasterFacade) triggerWatcher() {
	select {
	case f.watcherChanges <- struct{}{}:
	default:
		panic("migration watcher channel unexpectedly closed")
	}
}

func (f *stubMasterFacade) queueStatus(status coremigration.MigrationStatus) {
	f.status = append(f.status, status)
	f.triggerWatcher()
}

func (f *stubMasterFacade) triggerMinionReports() {
	select {
	case f.minionReportsChanges <- struct{}{}:
	default:
		panic("minion reports watcher channel unexpectedly closed")
	}
}

func (f *stubMasterFacade) queueMinionReports(r coremigration.MinionReports) {
	f.minionReports = append(f.minionReports, r)
	f.triggerMinionReports()
}

func (f *stubMasterFacade) Watch() (watcher.NotifyWatcher, error) {
	f.stub.AddCall("facade.Watch")
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	return newMockWatcher(f.watcherChanges), nil
}

func (f *stubMasterFacade) MigrationStatus() (coremigration.MigrationStatus, error) {
	f.stub.AddCall("facade.MigrationStatus")
	if f.statusErr != nil {
		return coremigration.MigrationStatus{}, f.statusErr
	}
	if len(f.status) == 0 {
		panic("no status queued to report")
	}
	out := f.status[0]
	f.status = f.status[1:]
	return out, nil
}

func (f *stubMasterFacade) WatchMinionReports() (watcher.NotifyWatcher, error) {
	f.stub.AddCall("facade.WatchMinionReports")
	if f.minionReportsWatchErr != nil {
		return nil, f.minionReportsWatchErr
	}
	return newMockWatcher(f.minionReportsChanges), nil
}

func (f *stubMasterFacade) MinionReports() (coremigration.MinionReports, error) {
	f.stub.AddCall("facade.MinionReports")
	if f.minionReportsErr != nil {
		return coremigration.MinionReports{}, f.minionReportsErr
	}
	if len(f.minionReports) == 0 {
		return coremigration.MinionReports{}, errors.NotFoundf("reports")

	}
	r := f.minionReports[0]
	f.minionReports = f.minionReports[1:]
	return r, nil
}

func (f *stubMasterFacade) MinionReportTimeout() (time.Duration, error) {
	f.stub.AddCall("facade.MinionReportTimeout")
	return f.minionReportTimeout, nil
}

func (f *stubMasterFacade) Prechecks() error {
	f.stub.AddCall("facade.Prechecks")
	return f.prechecksErr
}

func (f *stubMasterFacade) ModelInfo() (coremigration.ModelInfo, error) {
	f.stub.AddCall("facade.ModelInfo")
	if f.modelInfoErr != nil {
		return coremigration.ModelInfo{}, f.modelInfoErr
	}
	return coremigration.ModelInfo{
		UUID:             modelUUID,
		Name:             modelName,
		Owner:            ownerTag,
		AgentVersion:     modelVersion,
		ModelDescription: description.NewModel(description.ModelArgs{}),
	}, nil
}

func (f *stubMasterFacade) SourceControllerInfo() (coremigration.SourceControllerInfo, []string, error) {
	f.stub.AddCall("facade.SourceControllerInfo")
	return coremigration.SourceControllerInfo{
		ControllerTag:   sourceControllerTag,
		ControllerAlias: "mycontroller",
		Addrs:           []string{"source-addr"},
		CACert:          "cacert",
	}, []string{"related-model-uuid"}, nil
}

func (f *stubMasterFacade) Export() (coremigration.SerializedModel, error) {
	f.stub.AddCall("facade.Export")
	if f.exportErr != nil {
		return coremigration.SerializedModel{}, f.exportErr
	}
	return coremigration.SerializedModel{
		Bytes:  fakeModelBytes,
		Charms: []string{"charm0", "charm1"},
		Tools: map[version.Binary]string{
			version.MustParseBinary("2.1.0-ubuntu-amd64"): "/tools/0",
		},
		Resources: f.exportedResources,
	}, nil
}

func (f *stubMasterFacade) ProcessRelations(controllerAlias string) error {
	f.stub.AddCall("facade.ProcessRelations", controllerAlias)
	if f.processRelationsErr != nil {
		return f.processRelationsErr
	}
	return nil
}

func (f *stubMasterFacade) SetPhase(phase coremigration.Phase) error {
	f.stub.AddCall("facade.SetPhase", phase)
	return nil
}

func (f *stubMasterFacade) SetStatusMessage(message string) error {
	f.statuses = append(f.statuses, message)
	return nil
}

func (f *stubMasterFacade) Reap() error {
	f.stub.AddCall("facade.Reap")
	return nil
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
	c *gc.C
	api.Connection
	stub                *jujutesting.Stub
	prechecksErr        error
	importErr           error
	processRelationsErr error
	controllerTag       names.ControllerTag

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

func (c *stubConnection) APICall(objType string, _ int, _, request string, args, response interface{}) error {
	c.stub.AddCall(objType+"."+request, args)

	if objType == "MigrationTarget" {
		switch request {
		case "Prechecks":
			return c.prechecksErr
		case "Import":
			return c.importErr
		case "ProcessRelations":
			return c.processRelationsErr
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

func (c *stubConnection) ConnectControllerStream(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	c.stub.AddCall("ConnectControllerStream", path, attrs, headers)
	if c.streamErr != nil {
		return nil, c.streamErr
	}
	return c.logStream, nil
}

func makeStubUploadBinaries(stub *jujutesting.Stub) func(migration.UploadBinariesConfig) error {
	return func(config migration.UploadBinariesConfig) error {
		stub.AddCall(
			"UploadBinaries",
			config.Charms,
			config.CharmDownloader,
			config.Tools,
			config.ToolsDownloader,
			config.Resources,
			config.ResourceDownloader,
		)
		return nil
	}
}

// nullUploadBinaries is a UploadBinaries variant which is intended to
// not get called.
func nullUploadBinaries(migration.UploadBinariesConfig) error {
	panic("should not get called")
}

var fakeCharmDownloader = struct{ migration.CharmDownloader }{}

var fakeToolsDownloader = struct{ migration.ToolsDownloader }{}

func joinCalls(allCalls ...[]jujutesting.StubCall) (out []jujutesting.StubCall) {
	for _, calls := range allCalls {
		out = append(out, calls...)
	}
	return
}

func callNames(calls []jujutesting.StubCall) []string {
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
		SuccessCount: 5,
		UnknownCount: 0,
	}
}

type mockStream struct {
	base.Stream
	c          *gc.C
	written    []params.LogRecord
	writeErr   error
	closeCount int
}

func (s *mockStream) WriteJSON(v interface{}) error {
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
