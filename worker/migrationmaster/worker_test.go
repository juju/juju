// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"errors"
	"time"

	"launchpad.net/tomb"

	gc "gopkg.in/check.v1"

	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/migrationmaster"
)

type Suite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *Suite) TestWatchesForMigration(c *gc.C) {
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
	client.watcher.changes <- migration.TargetInfo{}

	// Worker should exit for now (TEMPORARY)
	select {
	case err := <-doneC:
		c.Assert(err, gc.ErrorMatches, "migration seen")
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to stop")
	}
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
}

func (c *mockClient) Watch() (watcher.MigrationMasterWatcher, error) {
	close(c.watchCalled)
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
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
