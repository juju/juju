// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/api"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/dependency"
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
	connCloseCall = jujutesting.StubCall{"Connection.Close", nil}
	abortCall     = jujutesting.StubCall{
		"APICall:MigrationTarget.Abort",
		[]interface{}{
			params.ModelArgs{ModelTag: names.NewModelTag("model-uuid").String()},
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
	w := migrationmaster.New(masterClient)
	s.triggerMigration(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration completed.
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"masterClient.SetPhase", []interface{}{migration.READONLY}},
		{"masterClient.SetPhase", []interface{}{migration.PRECHECK}},
		{"masterClient.SetPhase", []interface{}{migration.IMPORT}},
		{"masterClient.Export", nil},
		apiOpenCall,
		importCall,
		connCloseCall,
		{"masterClient.SetPhase", []interface{}{migration.VALIDATION}},
		{"masterClient.SetPhase", []interface{}{migration.SUCCESS}},
		{"masterClient.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterClient.SetPhase", []interface{}{migration.REAP}},
		{"masterClient.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestMigrationResume(c *gc.C) {
	// Test that a partially complete migration can be resumed.

	masterClient := newStubMasterClient(s.stub)
	w := migrationmaster.New(masterClient)
	masterClient.status.Phase = migration.VALIDATION
	s.triggerMigration(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
		{"masterClient.SetPhase", []interface{}{migration.SUCCESS}},
		{"masterClient.SetPhase", []interface{}{migration.LOGTRANSFER}},
		{"masterClient.SetPhase", []interface{}{migration.REAP}},
		{"masterClient.SetPhase", []interface{}{migration.DONE}},
	})
}

func (s *Suite) TestPreviouslyAbortedMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.status.Phase = migration.ABORTDONE
	s.triggerMigration(masterClient)
	w := migrationmaster.New(masterClient)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)

	// No reliable way to test stub calls in this case unfortunately.
}

func (s *Suite) TestPreviouslyCompletedMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.status.Phase = migration.DONE
	s.triggerMigration(masterClient)
	w := migrationmaster.New(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(errors.Cause(err), gc.Equals, dependency.ErrUninstall)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	client := newStubMasterClient(s.stub)
	client.watchErr = errors.New("boom")
	w := migrationmaster.New(client)
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "watching for migration: boom")
}

func (s *Suite) TestExportFailure(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	masterClient.exportErr = errors.New("boom")
	w := migrationmaster.New(masterClient)
	s.triggerMigration(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
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
	w := migrationmaster.New(masterClient)
	s.connectionErr = errors.New("boom")
	s.triggerMigration(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
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
	w := migrationmaster.New(masterClient)
	s.connection.importErr = errors.New("boom")
	s.triggerMigration(masterClient)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, migrationmaster.ErrDoneForNow)

	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.GetMigrationStatus", nil},
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
		changes: make(chan struct{}, 1),
	}
}

type mockWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func (w *mockWatcher) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *mockWatcher) Wait() error {
	return w.tomb.Wait()
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
		}
	}
	return errors.New("unexpected API call")
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}
