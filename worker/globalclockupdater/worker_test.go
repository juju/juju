// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/globalclock"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/globalclockupdater"
)

type WorkerSuite struct {
	testing.IsolationSuite
	stub       testing.Stub
	localClock *testclock.Clock
	updater    stubUpdater
	config     globalclockupdater.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.localClock = testclock.NewClock(time.Time{})
	s.updater = stubUpdater{
		added: make(chan time.Duration, 1),
	}
	s.config = globalclockupdater.Config{
		NewUpdater: func() (globalclock.Updater, error) {
			s.stub.AddCall("NewUpdater")
			return &s.updater, s.stub.NextErr()
		},
		LocalClock:     s.localClock,
		UpdateInterval: time.Second,
		BackoffDelay:   time.Minute,
	}
}

func (s *WorkerSuite) TestNewWorkerValidateNewUpdater(c *gc.C) {
	s.config.NewUpdater = nil
	s.testNewWorkerValidateConfig(c, "validating config: nil NewUpdater not valid")
}

func (s *WorkerSuite) TestNewWorkerValidateLocalClock(c *gc.C) {
	s.config.LocalClock = nil
	s.testNewWorkerValidateConfig(c, "validating config: nil LocalClock not valid")
}

func (s *WorkerSuite) TestNewWorkerValidateUpdateInterval(c *gc.C) {
	s.config.UpdateInterval = 0
	s.testNewWorkerValidateConfig(c, "validating config: non-positive UpdateInterval not valid")
}

func (s *WorkerSuite) TestNewWorkerValidateBackoffDelay(c *gc.C) {
	s.config.BackoffDelay = -1
	s.testNewWorkerValidateConfig(c, "validating config: non-positive BackoffDelay not valid")
}

func (s *WorkerSuite) testNewWorkerValidateConfig(c *gc.C, expect string) {
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *WorkerSuite) TestNewWorkerNewUpdaterError(c *gc.C) {
	s.stub.SetErrors(errors.New("nup"))
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, gc.ErrorMatches, "getting new updater: nup")
	c.Check(worker, gc.IsNil)
}

func (s *WorkerSuite) TestNewWorkerSuccess(c *gc.C) {
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer workertest.CleanKill(c, worker)
	s.stub.CheckCallNames(c, "NewUpdater")
}

func (s *WorkerSuite) TestWorkerUpdatesOnInterval(c *gc.C) {
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer workertest.CleanKill(c, worker)

	for i := 0; i < 2; i++ {
		s.localClock.WaitAdvance(500*time.Millisecond, time.Second, 1)
		select {
		case <-s.updater.added:
			c.Fatal("unexpected update")
		case <-time.After(coretesting.ShortWait):
		}

		s.localClock.WaitAdvance(501*time.Millisecond, time.Second, 1)
		select {
		case d := <-s.updater.added:
			c.Assert(d, gc.Equals, time.Second+time.Millisecond)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for update")
		}
	}
}

func (s *WorkerSuite) TestWorkerBackoffOnConcurrentUpdate(c *gc.C) {
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer workertest.CleanKill(c, worker)

	s.updater.SetErrors(globalclock.ErrConcurrentUpdate)

	s.localClock.WaitAdvance(time.Second, time.Second, 1)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, time.Second)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for update")
	}

	// The worker should be waiting for the backoff delay
	// before attempting another update.
	s.localClock.WaitAdvance(time.Second, time.Second, 1)
	select {
	case <-s.updater.added:
		c.Fatal("unexpected update")
	case <-time.After(coretesting.ShortWait):
	}

	s.localClock.WaitAdvance(59*time.Second, time.Second, 1)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, time.Minute)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for update")
	}
}

func (s *WorkerSuite) TestWorkerUpdateErrorStopsWorker(c *gc.C) {
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer workertest.DirtyKill(c, worker)

	s.updater.SetErrors(errors.New("burp"))
	s.localClock.WaitAdvance(time.Second, time.Second, 1)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "updating global clock: burp")
}

type stubUpdater struct {
	testing.Stub
	added chan time.Duration
}

func (s *stubUpdater) Advance(d time.Duration) error {
	s.MethodCall(s, "Advance", d)
	s.added <- d
	return s.NextErr()
}
