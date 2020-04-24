// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

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
		Logger:         loggo.GetLogger("globalclockupdater_test"),
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
		waitAdvance(c, s.localClock, 500*time.Millisecond)
		select {
		case <-s.updater.added:
			c.Fatal("unexpected update")
		case <-time.After(coretesting.ShortWait):
		}

		waitAdvance(c, s.localClock, 501*time.Millisecond)
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

	s.updater.SetErrors(errors.Annotate(globalclock.ErrConcurrentUpdate, "context info"))

	waitAdvance(c, s.localClock, time.Second)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, time.Second)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for update")
	}

	// The worker should be waiting for the backoff delay
	// before attempting another update.
	waitAdvance(c, s.localClock, time.Second)
	select {
	case <-s.updater.added:
		c.Fatal("unexpected update")
	case <-time.After(coretesting.ShortWait):
	}

	waitAdvance(c, s.localClock, 59*time.Second)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, time.Minute)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for update")
	}
}

func (s *WorkerSuite) TestWorkerHandlesTimeout(c *gc.C) {
	// At startup there's a time where the updater might return
	// timeouts - we should handle this cleanly.
	worker, err := globalclockupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	s.updater.SetErrors(errors.Annotate(globalclock.ErrTimeout, "some context"))

	waitAdvance(c, s.localClock, time.Second)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, time.Second)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for update")
	}

	// The worker should try again next time, adding the total missed
	// time.
	waitAdvance(c, s.localClock, time.Second)
	select {
	case d := <-s.updater.added:
		c.Assert(d, gc.Equals, 2*time.Second)
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
	waitAdvance(c, s.localClock, time.Second)
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "updating global clock: burp")
}

type stubUpdater struct {
	testing.Stub
	added chan time.Duration
}

func (s *stubUpdater) Advance(d time.Duration, _ <-chan struct{}) error {
	s.MethodCall(s, "Advance", d)
	s.added <- d
	return s.NextErr()
}

func waitAdvance(c *gc.C, clock *testclock.Clock, d time.Duration) {
	err := clock.WaitAdvance(d, time.Second, 1)
	c.Assert(err, jc.ErrorIsNil)
}
