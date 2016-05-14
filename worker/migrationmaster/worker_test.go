// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/workertest"
)

type Suite struct {
	coretesting.BaseSuite
	stub          *jujutesting.Stub
	connection    *stubConnection
	connectionErr error
}

var _ = gc.Suite(&Suite{})

var (
	fakeSerializedModel = []byte("model")
	modelTagString      = names.NewModelTag("model-uuid").String()

	// Define stub calls that commonly appear in tests here to allow reuse.
	apiOpenCall = jujutesting.StubCall{
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
	importCall = jujutesting.StubCall{
		"APICall:MigrationTarget.Import",
		[]interface{}{
			params.SerializedModel{Bytes: fakeSerializedModel},
		},
	}
	activateCall = jujutesting.StubCall{
		"APICall:MigrationTarget.Activate",
		[]interface{}{
			params.ModelArgs{ModelTag: modelTagString},
		},
	}
	connCloseCall = jujutesting.StubCall{"Connection.Close", nil}
	abortCall     = jujutesting.StubCall{
		"APICall:MigrationTarget.Abort",
		[]interface{}{
			params.ModelArgs{ModelTag: modelTagString},
		},
	}
)

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.connection = &stubConnection{stub: s.stub}
	s.connectionErr = nil
	s.PatchValue(migrationmaster.ApiOpen, s.apiOpen)
	s.PatchValue(migrationmaster.TempSuccessSleep, time.Millisecond)
}

func (s *Suite) apiOpen(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
	s.stub.AddCall("apiOpen", info, dialOpts)
	if s.connectionErr != nil {
		return nil, s.connectionErr
	}
	return s.connection, nil
}

func (s *Suite) triggerMigration(masterFacade *stubMasterFacade) {
	masterFacade.watcherChanges <- struct{}{}
}

func (s *Suite) TestSuccessfulMigration(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterFacade.SetPhase", []interface{}{migration.READONLY}},
		{"masterFacade.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterFacade.SetPhase", []interface{}{migration.IMPORT}},
		{"masterFacade.Export", nil},
		apiOpenCall,
		importCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{migration.VALIDATION}},
		apiOpenCall,
		activateCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{migration.SUCCESS}},
		{"masterFacade.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterFacade.SetPhase", []interface{}{migration.REAP}},
		{"masterFacade.Reap", nil},
		{"masterFacade.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.

	masterFacade := newStubMasterFacade(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	masterFacade.status.Phase = migration.SUCCESS
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterFacade.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterFacade.SetPhase", []interface{}{migration.REAP}},
		{"masterFacade.Reap", nil},
		{"masterFacade.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.status.Phase = migration.ABORTDONE
	s.triggerMigration(masterFacade)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)

	// No reliable way to test stub calls in this case unfortunately.
}

func (s *Suite) TestPreviouslyCompletedMigration(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.status.Phase = migration.DONE
	s.triggerMigration(masterFacade)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.watchErr = errors.New("boom")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "watching for migration: boom")
}

func (s *Suite) TestStatusError(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.statusErr = errors.New("splat")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
	})
}

func (s *Suite) TestStatusNotFound(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.statusErr = &params.Error{Code: params.CodeNotFound}
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Unlock", nil},
	})
}

func (s *Suite) TestUnlockError(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.statusErr = &params.Error{Code: params.CodeNotFound}
	guard := newStubGuard(s.stub)
	guard.unlockErr = errors.New("pow")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  guard,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "pow")

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Unlock", nil},
	})
}

func (s *Suite) TestLockdownError(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	guard := newStubGuard(s.stub)
	guard.lockdownErr = errors.New("biff")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  guard,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "biff")

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
	})
}

func (s *Suite) TestExportFailure(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	masterFacade.exportErr = errors.New("boom")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterFacade.SetPhase", []interface{}{migration.READONLY}},
		{"masterFacade.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterFacade.SetPhase", []interface{}{migration.IMPORT}},
		{"masterFacade.Export", nil},
		{"masterFacade.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		abortCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{migration.ABORTDONE}},
	})
}

func (s *Suite) TestAPIOpenFailure(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.connectionErr = errors.New("boom")
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterFacade.SetPhase", []interface{}{migration.READONLY}},
		{"masterFacade.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterFacade.SetPhase", []interface{}{migration.IMPORT}},
		{"masterFacade.Export", nil},
		apiOpenCall,
		{"masterFacade.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		{"masterFacade.SetPhase", []interface{}{migration.ABORTDONE}},
	})
}

func (s *Suite) TestImportFailure(c *gc.C) {
	masterFacade := newStubMasterFacade(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterFacade,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.connection.importErr = errors.New("boom")
	s.triggerMigration(masterFacade)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterFacade.Watch", nil},
		{"masterFacade.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterFacade.SetPhase", []interface{}{migration.READONLY}},
		{"masterFacade.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterFacade.SetPhase", []interface{}{migration.IMPORT}},
		{"masterFacade.Export", nil},
		apiOpenCall,
		importCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		abortCall,
		connCloseCall,
		{"masterFacade.SetPhase", []interface{}{migration.ABORTDONE}},
	})
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
		watcherChanges: make(chan struct{}, 1),
		status: masterapi.MigrationStatus{
			ModelUUID: "model-uuid",
			Attempt:   2,
			Phase:     migration.QUIESCE,
			TargetInfo: migration.TargetInfo{
				ControllerTag: names.NewModelTag("controller-uuid"),
				Addrs:         []string{"1.2.3.4:5"},
				CACert:        "cert",
				AuthTag:       names.NewUserTag("admin"),
				Password:      "secret",
			},
		},
	}
}

type stubMasterFacade struct {
	migrationmaster.Facade

	stub           *jujutesting.Stub
	watcherChanges chan struct{}
	watchErr       error
	status         masterapi.MigrationStatus
	statusErr      error
	exportErr      error
}

func (c *stubMasterFacade) Watch() (watcher.NotifyWatcher, error) {
	c.stub.AddCall("masterFacade.Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}

	return newMockWatcher(c.watcherChanges), nil
}

func (c *stubMasterFacade) GetMigrationStatus() (masterapi.MigrationStatus, error) {
	c.stub.AddCall("masterFacade.GetMigrationStatus")
	if c.statusErr != nil {
		return masterapi.MigrationStatus{}, c.statusErr
	}
	return c.status, nil
}

func (c *stubMasterFacade) Export() ([]byte, error) {
	c.stub.AddCall("masterFacade.Export")
	if c.exportErr != nil {
		return nil, c.exportErr
	}
	return fakeSerializedModel, nil
}

func (c *stubMasterFacade) SetPhase(phase migration.Phase) error {
	c.stub.AddCall("masterFacade.SetPhase", phase)
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
	stub      *jujutesting.Stub
	importErr error
}

func (c *stubConnection) BestFacadeVersion(string) int {
	return 1
}

func (c *stubConnection) APICall(objType string, version int, id, request string, params, response interface{}) error {
	c.stub.AddCall("APICall:"+objType+"."+request, params)

	if objType == "MigrationTarget" {
		switch request {
		case "Import":
			return c.importErr
		case "Activate":
			return nil
		}
	}
	return errors.New("unexpected API call")
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}
