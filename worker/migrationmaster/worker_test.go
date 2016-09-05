// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/migration"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/workertest"
)

type Suite struct {
	coretesting.BaseSuite
	clock         *jujutesting.Clock
	stub          *jujutesting.Stub
	connection    *stubConnection
	connectionErr error
	masterFacade  *stubMasterFacade
	config        migrationmaster.Config
}

var _ = gc.Suite(&Suite{})

var (
	fakeModelBytes = []byte("model")
	modelUUID      = "model-uuid"
	modelTag       = names.NewModelTag(modelUUID)
	modelTagString = modelTag.String()
	modelName      = "model-name"
	ownerTag       = names.NewUserTag("owner")
	modelVersion   = version.MustParse("1.2.4")

	// Define stub calls that commonly appear in tests here to allow reuse.
	apiOpenCallController = jujutesting.StubCall{
		"apiOpen",
		[]interface{}{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
			},
			api.DialOpts{},
		},
	}
	apiOpenCallModel = jujutesting.StubCall{
		"apiOpen",
		[]interface{}{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
				ModelTag: modelTag,
			},
			api.DialOpts{},
		},
	}
	importCall = jujutesting.StubCall{
		"MigrationTarget.Import",
		[]interface{}{
			params.SerializedModel{Bytes: fakeModelBytes},
		},
	}
	activateCall = jujutesting.StubCall{
		"MigrationTarget.Activate",
		[]interface{}{
			params.ModelArgs{ModelTag: modelTagString},
		},
	}
	connCloseCall = jujutesting.StubCall{"Connection.Close", nil}
	abortCall     = jujutesting.StubCall{
		"MigrationTarget.Abort",
		[]interface{}{
			params.ModelArgs{ModelTag: modelTagString},
		},
	}
	watchStatusLockdownCalls = []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.MigrationStatus", nil},
		{"guard.Lockdown", nil},
	}
	prechecksCalls = []jujutesting.StubCall{
		{"masterFacade.Prechecks", nil},
		{"masterFacade.ModelInfo", nil},
		apiOpenCallController,
		{"MigrationTarget.Prechecks", []interface{}{params.MigrationModelInfo{
			UUID:         modelUUID,
			Name:         modelName,
			OwnerTag:     ownerTag.String(),
			AgentVersion: modelVersion,
		}}},
		connCloseCall,
	}
	abortCalls = []jujutesting.StubCall{
		{"masterFacade.SetPhase", []interface{}{coremigration.ABORT}},
		apiOpenCallController,
		abortCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{coremigration.ABORTDONE}},
	}
)

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = jujutesting.NewClock(time.Now())
	s.stub = new(jujutesting.Stub)
	s.connection = &stubConnection{stub: s.stub}
	s.connectionErr = nil

	s.masterFacade = newStubMasterFacade(s.stub, s.clock.Now())

	// The default worker Config used by most of the tests. Tests may
	// tweak parts of this as needed.
	s.config = migrationmaster.Config{
		ModelUUID:       utils.MustNewUUID().String(),
		Facade:          s.masterFacade,
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

func (s *Suite) triggerMigration() {
	select {
	case s.masterFacade.watcherChanges <- struct{}{}:
	default:
		panic("migration watcher channel unexpectedly closed")
	}

}

func (s *Suite) triggerMinionReports() {
	select {
	case s.masterFacade.minionReportsChanges <- struct{}{}:
	default:
		panic("minion reports watcher channel unexpectedly closed")
	}
}

func (s *Suite) queueMinionReports(r coremigration.MinionReports) {
	s.masterFacade.minionReports = append(s.masterFacade.minionReports, r)
	s.triggerMinionReports()
}

func (s *Suite) queuePassingMinionReports(p coremigration.Phase) {
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId:  "model-uuid:2",
		Phase:        p,
		SuccessCount: 5,
		UnknownCount: 0,
	})
}

func (s *Suite) TestSuccessfulMigration(c *gc.C) {
	s.config.UploadBinaries = makeStubUploadBinaries(s.stub)
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()
	s.queuePassingMinionReports(coremigration.QUIESCE)
	s.queuePassingMinionReports(coremigration.VALIDATION)
	s.queuePassingMinionReports(coremigration.SUCCESS)

	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.Equals, migrationmaster.ErrMigrated)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,

		// QUIESCE
		prechecksCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.IMPORT}},

			//IMPORT
			{"masterFacade.Export", nil},
			apiOpenCallController,
			importCall,
			apiOpenCallModel,
			{"UploadBinaries", []interface{}{
				[]string{"charm0", "charm1"},
				fakeCharmDownloader,
				map[version.Binary]string{
					version.MustParseBinary("2.1.0-trusty-amd64"): "/tools/0",
				},
				fakeToolsDownloader,
			}},
			connCloseCall, // for target model
			connCloseCall, // for target controller
			{"masterFacade.SetPhase", []interface{}{coremigration.VALIDATION}},

			// VALIDATION
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			apiOpenCallController,
			activateCall,
			connCloseCall,
			{"masterFacade.SetPhase", []interface{}{coremigration.SUCCESS}},

			// SUCCESS
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},

			// LOGTRANSFER
			{"masterFacade.SetPhase", []interface{}{coremigration.REAP}},

			// REAP
			{"masterFacade.Reap", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.DONE}},
		}),
	)
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()
	s.queuePassingMinionReports(coremigration.SUCCESS)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, migrationmaster.ErrMigrated)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"masterFacade.SetPhase", []interface{}{coremigration.REAP}},
			{"masterFacade.Reap", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.ABORTDONE
	s.triggerMigration()

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	s.waitForStubCalls(c, []string{
		"masterFacade.Watch",
		"masterFacade.MigrationStatus",
		"guard.Unlock",
	})
}

func (s *Suite) TestPreviouslyCompletedMigration(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.DONE
	s.triggerMigration()
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.MigrationStatus", nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	s.masterFacade.watchErr = errors.New("boom")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "watching for migration: boom")
}

func (s *Suite) TestStatusError(c *gc.C) {
	s.masterFacade.statusErr = errors.New("splat")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.MigrationStatus", nil},
	})
}

func (s *Suite) TestStatusNotFound(c *gc.C) {
	s.masterFacade.statusErr = &params.Error{Code: params.CodeNotFound}
	s.triggerMigration()

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	s.waitForStubCalls(c, []string{
		"masterFacade.Watch",
		"masterFacade.MigrationStatus",
		"guard.Unlock",
	})
}

func (s *Suite) TestUnlockError(c *gc.C) {
	s.masterFacade.statusErr = &params.Error{Code: params.CodeNotFound}
	guard := newStubGuard(s.stub)
	guard.unlockErr = errors.New("pow")
	s.config.Guard = guard
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "pow")

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.MigrationStatus", nil},
		{"guard.Unlock", nil},
	})
}

func (s *Suite) TestLockdownError(c *gc.C) {
	guard := newStubGuard(s.stub)
	guard.lockdownErr = errors.New("biff")
	s.config.Guard = guard
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "biff")
	s.stub.CheckCalls(c, watchStatusLockdownCalls)
}

func (s *Suite) TestQUIESCEMinionWaitWatchError(c *gc.C) {
	s.checkMinionWaitWatchError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEMinionWaitGetError(c *gc.C) {
	s.checkMinionWaitGetError(c, coremigration.QUIESCE)
}

func (s *Suite) TestQUIESCEFailedAgent(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.QUIESCE

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.QUIESCE,
		FailedMachines: []string{"42"}, // a machine failed
	})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		prechecksCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCESourceChecksFail(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.QUIESCE
	s.masterFacade.prechecksErr = errors.New("boom")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{{"masterFacade.Prechecks", nil}},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCEModelInfoFail(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.QUIESCE
	s.masterFacade.modelInfoErr = errors.New("boom")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.Prechecks", nil},
			{"masterFacade.ModelInfo", nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCETargetChecksFail(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.QUIESCE
	s.connection.prechecksErr = errors.New("boom")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		prechecksCalls,
		abortCalls,
	))
}

func (s *Suite) TestExportFailure(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.IMPORT
	s.masterFacade.exportErr = errors.New("boom")
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.Export", nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestAPIOpenFailure(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.IMPORT
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.connectionErr = errors.New("boom")
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.Export", nil},
			apiOpenCallController,
			{"masterFacade.SetPhase", []interface{}{coremigration.ABORT}},
			apiOpenCallController,
			{"masterFacade.SetPhase", []interface{}{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestImportFailure(c *gc.C) {
	s.masterFacade.status.Phase = coremigration.IMPORT
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.connection.importErr = errors.New("boom")
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.Export", nil},
			apiOpenCallController,
			importCall,
			connCloseCall,
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
	s.masterFacade.status.Phase = coremigration.VALIDATION

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.VALIDATION,
		FailedMachines: []string{"42"}, // a machine failed
	})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
		},
		abortCalls,
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

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.SUCCESS,
		FailedMachines: []string{"42"},
	})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrMigrated)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"masterFacade.SetPhase", []interface{}{coremigration.REAP}},
			{"masterFacade.Reap", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitFailedUnit(c *gc.C) {
	// See note for TestMinionWaitFailedMachine above.

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId: "model-uuid:2",
		Phase:       coremigration.SUCCESS,
		FailedUnits: []string{"foo/2"},
	})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.MinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"masterFacade.SetPhase", []interface{}{coremigration.REAP}},
			{"masterFacade.Reap", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitTimeout(c *gc.C) {
	// The SUCCESS phase is special in that even if some minions fail
	// to report the migration should continue. There's no turning
	// back from SUCCESS.
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()

	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for clock.After call")
	}

	// Move time ahead in order to trigger timeout.
	s.clock.Advance(15 * time.Minute)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrMigrated)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"masterFacade.WatchMinionReports", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"masterFacade.SetPhase", []interface{}{coremigration.REAP}},
			{"masterFacade.Reap", nil},
			{"masterFacade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestMinionWaitWrongPhase(c *gc.C) {
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()

	// Have the phase in the minion reports be different from the
	// migration status. This shouldn't happen but the migrationmaster
	// should handle it.
	s.queuePassingMinionReports(coremigration.IMPORT)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, `minion reports phase \(IMPORT\) does not match migration phase \(SUCCESS\)`)
}

func (s *Suite) TestMinionWaitMigrationIdChanged(c *gc.C) {
	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.masterFacade.status.Phase = coremigration.SUCCESS
	s.triggerMigration()

	// Have the migration id in the minion reports be different from
	// the migration status. This shouldn't happen but the
	// migrationmaster should handle it.
	s.queueMinionReports(coremigration.MinionReports{
		MigrationId: "blah",
		Phase:       coremigration.SUCCESS,
	})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches,
		"unexpected migration id in minion reports, got blah, expected model-uuid:2")
}

func (s *Suite) TestAPIConnectWithMacaroon(c *gc.C) {
	// Set up macaroon based auth to the target.
	mac, err := macaroon.New([]byte("secret"), "id", "location")
	c.Assert(err, jc.ErrorIsNil)
	s.masterFacade.status.TargetInfo.Password = ""
	s.masterFacade.status.TargetInfo.Macaroon = mac

	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	s.masterFacade.status.Phase = coremigration.ABORT
	s.triggerMigration()

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrInactive)

	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{
				"apiOpen",
				[]interface{}{
					&api.Info{
						Addrs:     []string{"1.2.3.4:5"},
						CACert:    "cert",
						Tag:       names.NewUserTag("admin"),
						Macaroons: []macaroon.Slice{{mac}}, // <----
					},
					api.DialOpts{},
				},
			},
			abortCall,
			connCloseCall,
			{"masterFacade.SetPhase", []interface{}{coremigration.ABORTDONE}},
		},
	))
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
	s.masterFacade.minionReportsWatchErr = errors.New("boom")
	s.masterFacade.status.Phase = phase

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *Suite) checkMinionWaitGetError(c *gc.C, phase coremigration.Phase) {
	s.masterFacade.minionReportsErr = errors.New("boom")
	s.masterFacade.status.Phase = phase

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	s.triggerMigration()
	s.triggerMinionReports()

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "boom")
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

func newStubMasterFacade(stub *jujutesting.Stub, now time.Time) *stubMasterFacade {
	return &stubMasterFacade{
		stub:           stub,
		watcherChanges: make(chan struct{}, 999),
		status: coremigration.MigrationStatus{
			MigrationId:      "model-uuid:2",
			ModelUUID:        "model-uuid",
			Phase:            coremigration.QUIESCE,
			PhaseChangedTime: now,
			TargetInfo: coremigration.TargetInfo{
				ControllerTag: names.NewModelTag("controller-uuid"),
				Addrs:         []string{"1.2.3.4:5"},
				CACert:        "cert",
				AuthTag:       names.NewUserTag("admin"),
				Password:      "secret",
			},
		},

		// Give minionReportsChanges a larger-than-required buffer to
		// support waits at a number of phases.
		minionReportsChanges: make(chan struct{}, 999),
	}
}

type stubMasterFacade struct {
	migrationmaster.Facade

	stub *jujutesting.Stub

	watcherChanges chan struct{}
	watchErr       error
	status         coremigration.MigrationStatus
	statusErr      error

	prechecksErr error
	modelInfoErr error
	exportErr    error

	minionReportsChanges  chan struct{}
	minionReportsWatchErr error
	minionReports         []coremigration.MinionReports
	minionReportsErr      error
}

func (c *stubMasterFacade) Watch() (watcher.NotifyWatcher, error) {
	c.stub.AddCall("masterFacade.Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return newMockWatcher(c.watcherChanges), nil
}

func (c *stubMasterFacade) MigrationStatus() (coremigration.MigrationStatus, error) {
	c.stub.AddCall("masterFacade.MigrationStatus")
	if c.statusErr != nil {
		return coremigration.MigrationStatus{}, c.statusErr
	}
	return c.status, nil
}

func (c *stubMasterFacade) WatchMinionReports() (watcher.NotifyWatcher, error) {
	c.stub.AddCall("masterFacade.WatchMinionReports")
	if c.minionReportsWatchErr != nil {
		return nil, c.minionReportsWatchErr
	}
	return newMockWatcher(c.minionReportsChanges), nil
}

func (c *stubMasterFacade) MinionReports() (coremigration.MinionReports, error) {
	c.stub.AddCall("masterFacade.MinionReports")
	if c.minionReportsErr != nil {
		return coremigration.MinionReports{}, c.minionReportsErr
	}
	if len(c.minionReports) == 0 {
		return coremigration.MinionReports{}, errors.NotFoundf("reports")

	}
	r := c.minionReports[0]
	c.minionReports = c.minionReports[1:]
	return r, nil
}

func (c *stubMasterFacade) Prechecks() error {
	c.stub.AddCall("masterFacade.Prechecks")
	return c.prechecksErr
}

func (c *stubMasterFacade) ModelInfo() (coremigration.ModelInfo, error) {
	c.stub.AddCall("masterFacade.ModelInfo")
	if c.modelInfoErr != nil {
		return coremigration.ModelInfo{}, c.modelInfoErr
	}
	return coremigration.ModelInfo{
		UUID:         modelUUID,
		Name:         modelName,
		Owner:        ownerTag,
		AgentVersion: modelVersion,
	}, nil
}

func (c *stubMasterFacade) Export() (coremigration.SerializedModel, error) {
	c.stub.AddCall("masterFacade.Export")
	if c.exportErr != nil {
		return coremigration.SerializedModel{}, c.exportErr
	}
	return coremigration.SerializedModel{
		Bytes:  fakeModelBytes,
		Charms: []string{"charm0", "charm1"},
		Tools: map[version.Binary]string{
			version.MustParseBinary("2.1.0-trusty-amd64"): "/tools/0",
		},
	}, nil
}

func (c *stubMasterFacade) SetPhase(phase coremigration.Phase) error {
	c.stub.AddCall("masterFacade.SetPhase", phase)
	return nil
}

func (c *stubMasterFacade) SetStatusMessage(message string) error {
	return nil
}

func (c *stubMasterFacade) Reap() error {
	c.stub.AddCall("masterFacade.Reap")
	return nil
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
	api.Connection
	stub         *jujutesting.Stub
	prechecksErr error
	importErr    error
}

func (c *stubConnection) BestFacadeVersion(string) int {
	return 1
}

func (c *stubConnection) APICall(objType string, version int, id, request string, params, response interface{}) error {
	c.stub.AddCall(objType+"."+request, params)

	if objType == "MigrationTarget" {
		switch request {
		case "Prechecks":
			return c.prechecksErr
		case "Import":
			return c.importErr
		case "Activate":
			return nil
		}
	}
	return errors.New("unexpected API call")
}

func (c *stubConnection) Client() *api.Client {
	// This is kinda crappy but the *Client doesn't have to be
	// functional...
	return new(api.Client)
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}

func makeStubUploadBinaries(stub *jujutesting.Stub) func(migration.UploadBinariesConfig) error {
	return func(config migration.UploadBinariesConfig) error {
		stub.AddCall(
			"UploadBinaries",
			config.Charms,
			config.CharmDownloader,
			config.Tools,
			config.ToolsDownloader,
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
