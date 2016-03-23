// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"errors"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/workertest"
)

type Suite struct {
	coretesting.BaseSuite
	stub   *jujutesting.Stub
	client *stubMinionClient
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = new(jujutesting.Stub)
	s.client = newStubMinionClient(s.stub)
}

func (s *Suite) TestStartAndStop(c *gc.C) {
	w, err := migrationminion.New(s.client)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "Watch")
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	s.client.watchErr = errors.New("boom")
	w, err := migrationminion.New(s.client)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "setting up watcher: boom")
}

func newStubMinionClient(stub *jujutesting.Stub) *stubMinionClient {
	return &stubMinionClient{
		stub:    stub,
		watcher: newStubWatcher(),
	}
}

type stubMinionClient struct {
	stub     *jujutesting.Stub
	watcher  *stubWatcher
	watchErr error
}

func (c *stubMinionClient) Watch() (watcher.MigrationStatusWatcher, error) {
	c.stub.MethodCall(c, "Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func newStubWatcher() *stubWatcher {
	return &stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: make(chan params.MigrationStatus, 1),
	}
}

type stubWatcher struct {
	worker.Worker
	changes chan params.MigrationStatus
}

func (w *stubWatcher) Changes() <-chan params.MigrationStatus {
	return w.changes
}
