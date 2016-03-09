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

func (s *Suite) TestMigrationHandling(c *gc.C) {
	client := newMockClient()
	w := migrationmaster.New(client)

	doneC := make(chan error)
	go func() {
		doneC <- w.Wait()
	}()

	// Wait for Watch to be called
	select {
	case <-client.watchCalled:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for Watch to be called")
	}

	// Ensure worker is staying alive
	select {
	case <-doneC:
		c.Fatal("worker terminated prematurely")
	case <-time.After(coretesting.ShortWait):
	}

	// Trigger migration.
	client.watcher.changes <- migration.TargetInfo{
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

	// API connection should have been established and closed.
	expectedApiInfo := &api.Info{
		Addrs:    []string{"1.2.3.4:5"},
		CACert:   "cert",
		Tag:      names.NewUserTag("admin"),
		Password: "secret",
	}
	s.stub.CheckCalls(c, []jujutesting.StubCall{
		{"apiOpen", []interface{}{expectedApiInfo, api.DefaultDialOpts()}},
		{"Connection.Close", []interface{}{}},
	})

	// The migration should have been aborted.
	c.Assert(client.phaseSet, gc.Equals, migration.ABORT)
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	client := newMockClient()
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

func newMockClient() *mockClient {
	return &mockClient{
		watchCalled: make(chan bool),
		watcher:     newMockWatcher(),
	}
}

type mockClient struct {
	masterapi.Client
	watchCalled chan bool
	watchErr    error
	watcher     *mockWatcher
	phaseSet    migration.Phase
}

func (c *mockClient) Watch() (watcher.MigrationMasterWatcher, error) {
	close(c.watchCalled)
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func (c *mockClient) SetPhase(phase migration.Phase) error {
	c.phaseSet = phase
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

func (c *stubConnection) Close() error {
	c.stub.AddCall("Connection.Close")
	return nil
}
