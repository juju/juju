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

func (s *Suite) triggerMigration(masterClient *stubMasterClient) {
	masterClient.watcher.changes <- struct{}{}
}

func (s *Suite) TestSuccessfulMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterClient.SetPhase", []interface{}{migration.READONLY}},
		{"masterClient.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterClient.SetPhase", []interface{}{migration.IMPORT}},
		{"masterClient.Export", nil},
		apiOpenCall,
		importCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.VALIDATION}},
		apiOpenCall,
		activateCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.SUCCESS}},
		{"masterClient.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterClient.SetPhase", []interface{}{migration.REAP}},
		{"masterClient.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.

	masterClient := newStubMasterClient(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	masterClient.status.Phase = migration.SUCCESS
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterClient.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterClient.SetPhase", []interface{}{migration.REAP}},
		{"masterClient.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.status.Phase = migration.ABORTDONE
	s.triggerMigration(masterClient)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)

	// No reliable way to test stub calls in this case unfortunately.
}

func (s *Suite) TestPreviouslyCompletedMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.status.Phase = migration.DONE
	s.triggerMigration(masterClient)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.watchErr = errors.New("boom")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "watching for migration: boom")
}

func (s *Suite) TestStatusError(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.statusErr = errors.New("splat")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
	})
}

func (s *Suite) TestStatusNotFound(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.statusErr = &params.Error{Code: params.CodeNotFound}
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Unlock", nil},
	})
}

func (s *Suite) TestUnlockError(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.statusErr = &params.Error{Code: params.CodeNotFound}
	guard := newStubGuard(s.stub)
	guard.unlockErr = errors.New("pow")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  guard,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "pow")

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Unlock", nil},
	})
}

func (s *Suite) TestLockdownError(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	guard := newStubGuard(s.stub)
	guard.lockdownErr = errors.New("biff")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  guard,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "biff")

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
	})
}

func (s *Suite) TestExportFailure(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.exportErr = errors.New("boom")
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterClient.SetPhase", []interface{}{migration.READONLY}},
		{"masterClient.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterClient.SetPhase", []interface{}{migration.IMPORT}},
		{"masterClient.Export", nil},
		{"masterClient.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		abortCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.ABORTDONE}},
	})
}

func (s *Suite) TestAPIOpenFailure(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.connectionErr = errors.New("boom")
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterClient.SetPhase", []interface{}{migration.READONLY}},
		{"masterClient.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterClient.SetPhase", []interface{}{migration.IMPORT}},
		{"masterClient.Export", nil},
		apiOpenCall,
		{"masterClient.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		{"masterClient.SetPhase", []interface{}{migration.ABORTDONE}},
	})
}

func (s *Suite) TestImportFailure(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	worker, err := migrationmaster.New(migrationmaster.Config{
		Facade: masterClient,
		Guard:  newStubGuard(s.stub),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.connection.importErr = errors.New("boom")
	s.triggerMigration(masterClient)

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"guard.Lockdown", nil},
		{"masterClient.SetPhase", []interface{}{migration.READONLY}},
		{"masterClient.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterClient.SetPhase", []interface{}{migration.IMPORT}},
		{"masterClient.Export", nil},
		apiOpenCall,
		importCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.ABORT}},
		apiOpenCall,
		abortCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.ABORTDONE}},
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

func newStubMasterClient(stub *jujutesting.Stub) *stubMasterClient {
	return &stubMasterClient{
		stub:    stub,
		watcher: newMockWatcher(),
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

type stubMasterClient struct {
	masterapi.Client
	stub      *jujutesting.Stub
	watcher   *mockWatcher
	watchErr  error
	status    masterapi.MigrationStatus
	statusErr error
	exportErr error
}

func (c *stubMasterClient) Watch() (watcher.NotifyWatcher, error) {
	c.stub.AddCall("masterClient.Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func (c *stubMasterClient) GetMigrationStatus() (masterapi.MigrationStatus, error) {
	c.stub.AddCall("masterClient.GetMigrationStatus")
	if c.statusErr != nil {
		return masterapi.MigrationStatus{}, c.statusErr
	}
	return c.status, nil
}

func (c *stubMasterClient) Export() ([]byte, error) {
	c.stub.AddCall("masterClient.Export")
	if c.exportErr != nil {
		return nil, c.exportErr
	}
	return fakeSerializedModel, nil
}

func (c *stubMasterClient) SetPhase(phase migration.Phase) error {
	c.stub.AddCall("masterClient.SetPhase", phase)
	return nil
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: make(chan struct{}, 1),
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
