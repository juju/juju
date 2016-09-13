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
	facade        *stubMasterFacade
	config        migrationmaster.Config
}

var _ = gc.Suite(&Suite{})

var (
	fakeModelBytes      = []byte("model")
	targetControllerTag = names.NewControllerTag("controller-uuid")
	modelUUID           = "model-uuid"
	modelTag            = names.NewModelTag(modelUUID)
	modelName           = "model-name"
	ownerTag            = names.NewUserTag("owner")
	modelVersion        = version.MustParse("1.2.4")

	// Define stub calls that commonly appear in tests here to allow reuse.
	apiOpenControllerCall = jujutesting.StubCall{
		"apiOpen",
		[]interface{}{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
			},
			migration.ControllerDialOpts(),
		},
	}
	apiOpenModelCall = jujutesting.StubCall{
		"apiOpen",
		[]interface{}{
			&api.Info{
				Addrs:    []string{"1.2.3.4:5"},
				CACert:   "cert",
				Tag:      names.NewUserTag("admin"),
				Password: "secret",
				ModelTag: modelTag,
			},
			migration.ControllerDialOpts(),
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
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	apiCloseCall = jujutesting.StubCall{"Connection.Close", nil}
	abortCall    = jujutesting.StubCall{
		"MigrationTarget.Abort",
		[]interface{}{
			params.ModelArgs{ModelTag: modelTag.String()},
		},
	}
	watchStatusLockdownCalls = []jujutesting.StubCall{
		{"facade.Watch", nil},
		{"facade.MigrationStatus", nil},
		{"guard.Lockdown", nil},
	}
	prechecksCalls = []jujutesting.StubCall{
		{"facade.Prechecks", nil},
		{"facade.ModelInfo", nil},
		apiOpenControllerCall,
		{"MigrationTarget.Prechecks", []interface{}{params.MigrationModelInfo{
			UUID:         modelUUID,
			Name:         modelName,
			OwnerTag:     ownerTag.String(),
			AgentVersion: modelVersion,
		}}},
		apiCloseCall,
	}
	abortCalls = []jujutesting.StubCall{
		{"facade.SetPhase", []interface{}{coremigration.ABORT}},
		apiOpenControllerCall,
		abortCall,
		apiCloseCall,
		{"facade.SetPhase", []interface{}{coremigration.ABORTDONE}},
	}
)

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = jujutesting.NewClock(time.Now())
	s.stub = new(jujutesting.Stub)
	s.connection = &stubConnection{
		stub:          s.stub,
		controllerTag: targetControllerTag,
	}
	s.connectionErr = nil

	s.facade = newStubMasterFacade(s.stub, s.clock.Now())

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
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.QUIESCE))
	s.facade.queueMinionReports(makeMinionReports(coremigration.VALIDATION))
	s.facade.queueMinionReports(makeMinionReports(coremigration.SUCCESS))
	s.config.UploadBinaries = makeStubUploadBinaries(s.stub)

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,

		// QUIESCE
		prechecksCalls,
		[]jujutesting.StubCall{
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.IMPORT}},

			//IMPORT
			{"facade.Export", nil},
			apiOpenControllerCall,
			importCall,
			apiOpenModelCall,
			{"UploadBinaries", []interface{}{
				[]string{"charm0", "charm1"},
				fakeCharmDownloader,
				map[version.Binary]string{
					version.MustParseBinary("2.1.0-trusty-amd64"): "/tools/0",
				},
				fakeToolsDownloader,
			}},
			apiCloseCall, // for target model
			apiCloseCall, // for target controller
			{"facade.SetPhase", []interface{}{coremigration.VALIDATION}},

			// VALIDATION
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			apiOpenControllerCall,
			activateCall,
			apiCloseCall,
			{"facade.SetPhase", []interface{}{coremigration.SUCCESS}},

			// SUCCESS
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},

			// LOGTRANSFER
			{"facade.SetPhase", []interface{}{coremigration.REAP}},

			// REAP
			{"facade.Reap", nil},
			{"facade.SetPhase", []interface{}{coremigration.DONE}},
		}),
	)
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.facade.queueMinionReports(makeMinionReports(coremigration.SUCCESS))

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"facade.SetPhase", []interface{}{coremigration.REAP}},
			{"facade.Reap", nil},
			{"facade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.ABORTDONE))

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

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
		{"facade.Watch", nil},
		{"facade.MigrationStatus", nil},
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
		{"facade.Watch", nil},
		{"facade.MigrationStatus", nil},
	})
}

func (s *Suite) TestStatusNotFound(c *gc.C) {
	s.facade.statusErr = &params.Error{Code: params.CodeNotFound}
	s.facade.triggerWatcher()

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

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
		{"facade.Watch", nil},
		{"facade.MigrationStatus", nil},
		{"guard.Unlock", nil},
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
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		prechecksCalls,
		[]jujutesting.StubCall{
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCEWrongController(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.controllerTag = names.NewControllerTag("another-controller")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"facade.Prechecks", nil},
			{"facade.ModelInfo", nil},
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
		[]jujutesting.StubCall{{"facade.Prechecks", nil}},
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
			{"facade.Prechecks", nil},
			{"facade.ModelInfo", nil},
		},
		abortCalls,
	))
}

func (s *Suite) TestQUIESCETargetChecksFail(c *gc.C) {
	s.facade.queueStatus(s.makeStatus(coremigration.QUIESCE))
	s.connection.prechecksErr = errors.New("boom")

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		prechecksCalls,
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
			{"facade.Export", nil},
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
			{"facade.Export", nil},
			apiOpenControllerCall,
			{"facade.SetPhase", []interface{}{coremigration.ABORT}},
			apiOpenControllerCall,
			{"facade.SetPhase", []interface{}{coremigration.ABORTDONE}},
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
			{"facade.Export", nil},
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
	s.facade.queueStatus(s.makeStatus(coremigration.VALIDATION))
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId:    "model-uuid:2",
		Phase:          coremigration.VALIDATION,
		FailedMachines: []string{"42"}, // a machine failed
	})

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
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
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"facade.SetPhase", []interface{}{coremigration.REAP}},
			{"facade.Reap", nil},
			{"facade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitFailedUnit(c *gc.C) {
	// See note for TestMinionWaitFailedMachine above.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))
	s.facade.queueMinionReports(coremigration.MinionReports{
		MigrationId: "model-uuid:2",
		Phase:       coremigration.SUCCESS,
		FailedUnits: []string{"foo/2"},
	})

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		watchStatusLockdownCalls,
		[]jujutesting.StubCall{
			{"facade.WatchMinionReports", nil},
			{"facade.MinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"facade.SetPhase", []interface{}{coremigration.REAP}},
			{"facade.Reap", nil},
			{"facade.SetPhase", []interface{}{coremigration.DONE}},
		},
	))
}

func (s *Suite) TestSUCCESSMinionWaitTimeout(c *gc.C) {
	// The SUCCESS phase is special in that even if some minions fail
	// to report the migration should continue. There's no turning
	// back from SUCCESS.
	s.facade.queueStatus(s.makeStatus(coremigration.SUCCESS))

	worker, err := migrationmaster.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

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
			{"facade.WatchMinionReports", nil},
			{"facade.SetPhase", []interface{}{coremigration.LOGTRANSFER}},
			{"facade.SetPhase", []interface{}{coremigration.REAP}},
			{"facade.Reap", nil},
			{"facade.SetPhase", []interface{}{coremigration.DONE}},
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

func (s *Suite) TestAPIConnectWithMacaroon(c *gc.C) {
	// Use ABORT because it involves an API connection to the target
	// and is convenient.
	status := s.makeStatus(coremigration.ABORT)

	// Set up macaroon based auth to the target.
	mac, err := macaroon.New([]byte("secret"), "id", "location")
	c.Assert(err, jc.ErrorIsNil)
	macs := []macaroon.Slice{{mac}}
	status.TargetInfo.Password = ""
	status.TargetInfo.Macaroons = macs

	s.facade.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
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
						Macaroons: macs, // <---
					},
					migration.ControllerDialOpts(),
				},
			},
			abortCall,
			apiCloseCall,
			{"facade.SetPhase", []interface{}{coremigration.ABORTDONE}},
		},
	))
}

func (s *Suite) TestExternalControl(c *gc.C) {
	status := s.makeStatus(coremigration.QUIESCE)
	status.ExternalControl = true
	s.facade.queueStatus(status)

	status.Phase = coremigration.DONE
	s.facade.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrMigrated)
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,

		// Wait for migration to end.
		[]jujutesting.StubCall{
			{"facade.Watch", nil},
			{"facade.MigrationStatus", nil},
		},
	))
}

func (s *Suite) TestExternalControlABORT(c *gc.C) {
	status := s.makeStatus(coremigration.QUIESCE)
	status.ExternalControl = true
	s.facade.queueStatus(status)

	status.Phase = coremigration.ABORTDONE
	s.facade.queueStatus(status)

	s.checkWorkerReturns(c, migrationmaster.ErrInactive)
	s.stub.CheckCalls(c, joinCalls(
		// Wait for migration to start.
		watchStatusLockdownCalls,

		// Wait for migration to end.
		[]jujutesting.StubCall{
			{"facade.Watch", nil},
			{"facade.MigrationStatus", nil},
		},
	))
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
	status         []coremigration.MigrationStatus
	statusErr      error

	prechecksErr error
	modelInfoErr error
	exportErr    error

	minionReportsChanges  chan struct{}
	minionReportsWatchErr error
	minionReports         []coremigration.MinionReports
	minionReportsErr      error
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
		UUID:         modelUUID,
		Name:         modelName,
		Owner:        ownerTag,
		AgentVersion: modelVersion,
	}, nil
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
			version.MustParseBinary("2.1.0-trusty-amd64"): "/tools/0",
		},
	}, nil
}

func (f *stubMasterFacade) SetPhase(phase coremigration.Phase) error {
	f.stub.AddCall("facade.SetPhase", phase)
	return nil
}

func (f *stubMasterFacade) SetStatusMessage(message string) error {
	return nil
}

func (f *stubMasterFacade) Reap() error {
	f.stub.AddCall("facade.Reap")
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
	stub          *jujutesting.Stub
	prechecksErr  error
	importErr     error
	controllerTag names.ControllerTag
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

func (c *stubConnection) ControllerTag() names.ControllerTag {
	return c.controllerTag
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

func makeMinionReports(p coremigration.Phase) coremigration.MinionReports {
	return coremigration.MinionReports{
		MigrationId:  "model-uuid:2",
		Phase:        p,
		SuccessCount: 5,
		UnknownCount: 0,
	}
}
