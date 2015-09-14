// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/statushistorypruner"
)

type statusHistoryPrunerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&statusHistoryPrunerSuite{})

func (s *statusHistoryPrunerSuite) TestWorkerCallsPrune(c *gc.C) {
	fakeTimer := newMockTimer(coretesting.LongWait)

	fakeTimerFunc := func(d time.Duration) worker.PeriodicTimer {
		// construction of timer should be with 0 because we intend it to
		// run once before waiting.
		c.Assert(d, gc.Equals, 0*time.Nanosecond)
		return fakeTimer
	}
	facade := newFakeFacade()
	conf := statushistorypruner.Config{
		Facade:           facade,
		MaxLogsPerEntity: 3,
		PruneInterval:    coretesting.ShortWait,
		NewTimer:         fakeTimerFunc,
	}

	pruner, err := statushistorypruner.New(conf)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		c.Assert(worker.Stop(pruner), jc.ErrorIsNil)
	})

	err = fakeTimer.fire()
	c.Check(err, jc.ErrorIsNil)

	var passedLogs int
	select {
	case passedLogs = <-facade.passedMaxLogs:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for passed logs to pruner")
	}
	c.Assert(passedLogs, gc.Equals, 3)

	// Reset will have been called with the actual PruneInterval
	var period time.Duration
	select {
	case period = <-fakeTimer.period:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for period reset by pruner")
	}
	c.Assert(period, gc.Equals, coretesting.ShortWait)
}

func (s *statusHistoryPrunerSuite) TestWorkerWontCallPruneBeforeFiringTimer(c *gc.C) {
	fakeTimer := newMockTimer(coretesting.LongWait)

	fakeTimerFunc := func(d time.Duration) worker.PeriodicTimer {
		// construction of timer should be with 0 because we intend it to
		// run once before waiting.
		c.Assert(d, gc.Equals, 0*time.Nanosecond)
		return fakeTimer
	}
	facade := newFakeFacade()
	conf := statushistorypruner.Config{
		Facade:           facade,
		MaxLogsPerEntity: 3,
		PruneInterval:    coretesting.ShortWait,
		NewTimer:         fakeTimerFunc,
	}

	pruner, err := statushistorypruner.New(conf)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		c.Assert(worker.Stop(pruner), jc.ErrorIsNil)
	})

	select {
	case <-facade.passedMaxLogs:
		c.Fatal("called before firing timer.")
	case <-time.After(coretesting.LongWait):
	}
}

type mockTimer struct {
	period chan time.Duration
	c      chan time.Time
}

func (t *mockTimer) Reset(d time.Duration) bool {
	select {
	case t.period <- d:
	case <-time.After(coretesting.LongWait):
		panic("timed out waiting for timer to reset")
	}
	return true
}

func (t *mockTimer) CountDown() <-chan time.Time {
	return t.c
}

func (t *mockTimer) fire() error {
	select {
	case t.c <- time.Time{}:
	case <-time.After(coretesting.LongWait):
		return errors.New("timed out waiting for pruner to run")
	}
	return nil
}

func newMockTimer(d time.Duration) *mockTimer {
	return &mockTimer{period: make(chan time.Duration, 1),
		c: make(chan time.Time),
	}
}

type fakeFacade struct {
	passedMaxLogs chan int
}

func newFakeFacade() *fakeFacade {
	return &fakeFacade{
		passedMaxLogs: make(chan int, 1),
	}
}

// Prune implements Facade
func (f *fakeFacade) Prune(maxLogs int) error {
	select {
	case f.passedMaxLogs <- maxLogs:
	case <-time.After(coretesting.LongWait):
		return errors.New("timed out waiting for facade call Prune to run")
	}
	return nil
}
