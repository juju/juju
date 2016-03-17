// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"errors"
	"time"

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
	"github.com/juju/juju/worker/migrationmaster"
)

type Suite struct {
	coretesting.BaseSuite
	stub       *jujutesting.Stub
	connection stubConnection
}

var _ = gc.Suite(&Suite{})

var fakeSerializedModel = []byte("model")

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.connection.stub = s.stub
	s.PatchValue(migrationmaster.ApiOpen, s.apiOpen)
}

func (s *Suite) apiOpen(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
	s.stub.AddCall("apiOpen", info, dialOpts)
	return &s.connection, nil
}

func (s *Suite) TestMigration(c *gc.C) {
	masterClient := newStubMasterClient(s.stub)
	w := migrationmaster.New(masterClient)

	doneC := make(chan error)
	go func() {
		doneC <- w.Wait()
	}()

	// Trigger migration.
	masterClient.watcher.changes <- migration.TargetInfo{
		ControllerTag: names.NewModelTag("uuid"),
		Addrs:         []string{"1.2.3.4:5"},
		CACert:        "cert",
		AuthTag:       names.NewUserTag("admin"),
		Password:      "secret",
	}

	// Worker should exit for now (TEMPORARY)
	select {
	case err := <-doneC:
		c.Assert(err, gc.ErrorMatches, "migration seen and aborted")
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to stop")
	}

	// Observe that the migration was seen, the model exported, an API
	// connection to the target controller was made, the model was
	// imported and then the migration aborted.
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"masterClient.Watch", nil},
		{"masterClient.Export", nil},
		{"apiOpen", []interface{}{&api.Info{
			Addrs:    []string{"1.2.3.4:5"},
			CACert:   "cert",
			Tag:      names.NewUserTag("admin"),
			Password: "secret",
		}, api.DefaultDialOpts()}},
		{"APICall:MigrationTarget.Import",
			[]interface{}{params.SerializedModel{Bytes: fakeSerializedModel}}},
		{"masterClient.SetPhase", []interface{}{migration.ABORT}},
		{"Connection.Close", nil},
	})
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	client := newStubMasterClient(s.stub)
	client.watchErr = errors.New("boom")
	w := migrationmaster.New(client)

	doneC := make(chan error)
	go func() {
		doneC <- w.Wait()
	}()

	select {
	case err := <-doneC:
		c.Assert(err, gc.ErrorMatches, "watching for migration: boom")
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to stop")
	}
}

func newStubMasterClient(stub *jujutesting.Stub) *stubMasterClient {
	return &stubMasterClient{
		stub:    stub,
		watcher: newMockWatcher(),
	}
}

type stubMasterClient struct {
	masterapi.Client
	stub     *jujutesting.Stub
	watcher  *mockWatcher
	watchErr error
}

func (c *stubMasterClient) Watch() (watcher.MigrationMasterWatcher, error) {
	c.stub.AddCall("masterClient.Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func (c *stubMasterClient) Export() ([]byte, error) {
	c.stub.AddCall("masterClient.Export")
	return fakeSerializedModel, nil
}

func (c *stubMasterClient) SetPhase(phase migration.Phase) error {
	c.stub.AddCall("masterClient.SetPhase", phase)
	return nil
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		changes: make(chan migration.TargetInfo, 1),
	}
}

type mockWatcher struct {
	tomb    tomb.Tomb
	changes chan migration.TargetInfo
}

func (w *mockWatcher) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *mockWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *mockWatcher) Changes() <-chan migration.TargetInfo {
	return w.changes
}

type stubConnection struct {
	api.Connection
	stub *jujutesting.Stub
}

func (c *stubConnection) BestFacadeVersion(string) int {
	return 1
}

func (c *stubConnection) APICall(objType string, version int, id, request string, params, response interface{}) error {
	c.stub.AddCall("APICall:"+objType+"."+request, params)

	if objType == "MigrationTarget" {
		switch request {
		case "Import":
			return nil
		}
	}
	return errors.New("unexpected API call")
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}
