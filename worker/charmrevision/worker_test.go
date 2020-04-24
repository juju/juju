// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/charmrevision"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestUpdatesImmediately(c *gc.C) {
	fix := newFixture(time.Minute)
	fix.cleanTest(c, func(_ worker.Worker) {
		fix.waitCall(c)
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions")
}

func (s *WorkerSuite) TestNoMoreUpdatesUntilPeriod(c *gc.C) {
	fix := newFixture(time.Minute)
	fix.cleanTest(c, func(_ worker.Worker) {
		fix.waitCall(c)
		fix.clock.Advance(time.Minute - time.Nanosecond)
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions")
}

func (s *WorkerSuite) TestUpdatesAfterPeriod(c *gc.C) {
	fix := newFixture(time.Minute)
	fix.cleanTest(c, func(_ worker.Worker) {
		fix.waitCall(c)
		if err := fix.clock.WaitAdvance(time.Minute, testing.LongWait, 1); err != nil {
			c.Fatal(err)
		}
		fix.waitCall(c)
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions", "UpdateLatestRevisions")
}

func (s *WorkerSuite) TestImmediateUpdateError(c *gc.C) {
	fix := newFixture(time.Minute)
	fix.revisionUpdater.stub.SetErrors(
		errors.New("no updates for you"),
	)
	fix.dirtyTest(c, func(w worker.Worker) {
		fix.waitCall(c)
		c.Check(w.Wait(), gc.ErrorMatches, "no updates for you")
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions")
}

func (s *WorkerSuite) TestDelayedUpdateError(c *gc.C) {
	fix := newFixture(time.Minute)
	fix.revisionUpdater.stub.SetErrors(
		nil,
		errors.New("no more updates for you"),
	)
	fix.dirtyTest(c, func(w worker.Worker) {
		fix.waitCall(c)
		if err := fix.clock.WaitAdvance(time.Minute, testing.LongWait, 1); err != nil {
			c.Fatal(err)
		}
		fix.waitCall(c)
		c.Check(w.Wait(), gc.ErrorMatches, "no more updates for you")
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions", "UpdateLatestRevisions")
}

// workerFixture isolates a charmrevision worker for testing.
type workerFixture struct {
	revisionUpdater mockRevisionUpdater
	clock           *testclock.Clock
	period          time.Duration
}

func newFixture(period time.Duration) workerFixture {
	return workerFixture{
		revisionUpdater: newMockRevisionUpdater(),
		clock:           testclock.NewClock(coretesting.ZeroTime()),
		period:          period,
	}
}

type testFunc func(worker.Worker)

func (fix workerFixture) cleanTest(c *gc.C, test testFunc) {
	fix.runTest(c, test, true)
}

func (fix workerFixture) dirtyTest(c *gc.C, test testFunc) {
	fix.runTest(c, test, false)
}

func (fix workerFixture) runTest(c *gc.C, test testFunc, checkWaitErr bool) {
	w, err := charmrevision.NewWorker(charmrevision.Config{
		RevisionUpdater: fix.revisionUpdater,
		Clock:           fix.clock,
		Period:          fix.period,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := worker.Stop(w)
		if checkWaitErr {
			c.Check(err, jc.ErrorIsNil)
		}
	}()
	test(w)
}

func (fix workerFixture) waitCall(c *gc.C) {
	select {
	case <-fix.revisionUpdater.calls:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (fix workerFixture) waitNoCall(c *gc.C) {
	select {
	case <-fix.revisionUpdater.calls:
		c.Fatalf("unexpected revisionUpdater call")
	case <-time.After(coretesting.ShortWait):
	}
}

// mockRevisionUpdater records (and notifies of) calls made to UpdateLatestRevisions.
type mockRevisionUpdater struct {
	stub  *testing.Stub
	calls chan struct{}
}

func newMockRevisionUpdater() mockRevisionUpdater {
	return mockRevisionUpdater{
		stub:  &testing.Stub{},
		calls: make(chan struct{}, 1000),
	}
}

func (mock mockRevisionUpdater) UpdateLatestRevisions() error {
	mock.stub.AddCall("UpdateLatestRevisions")
	mock.calls <- struct{}{}
	return mock.stub.NextErr()
}
