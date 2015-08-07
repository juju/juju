// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/statushistorypruner"
)

type Timer struct {
	period time.Duration
	c      chan time.Time
	gcC    *gc.C
}

func (t *Timer) Reset(d time.Duration) bool {
	t.period = d
	return true
}

func (t *Timer) C() <-chan time.Time {
	return t.c
}

func (t *Timer) Fire() {
	select {
	case t.c <- time.Time{}:
	case <-time.After(coretesting.LongWait):
		t.gcC.Fatalf("timed out waiting for pruner to run")
	}
}

func NewTimer(d time.Duration, c *gc.C) worker.PeriodicTimer {
	return &Timer{period: d,
		c:   make(chan time.Time),
		gcC: c}
}

var _ = gc.Suite(&StatusHistoryPrunerSuite{})

type StatusHistoryPrunerSuite struct {
	coretesting.BaseSuite
}

func (s *StatusHistoryPrunerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *StatusHistoryPrunerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *StatusHistoryPrunerSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *StatusHistoryPrunerSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *StatusHistoryPrunerSuite) TestWorker(c *gc.C) {
	var passedMaxLogs int
	fakePruner := func(_ *state.State, maxLogs int) error {
		passedMaxLogs = maxLogs
		return nil
	}
	params := statushistorypruner.HistoryPrunerParams{
		MaxLogsPerState: 3,
		PruneInterval:   coretesting.ShortWait,
	}
	fakeTimer := NewTimer(coretesting.LongWait, c)

	fakeTimerFunc := func(d time.Duration) worker.PeriodicTimer {
		// construction of timer should be with 0 because we intend it to
		// run once before waiting.
		c.Assert(d, gc.Equals, 0*time.Nanosecond)
		return fakeTimer
	}
	pruner := statushistorypruner.NewForTests(
		&state.State{},
		&params,
		fakeTimerFunc,
		fakePruner,
	)
	s.AddCleanup(func(*gc.C) {
		pruner.Kill()
		c.Assert(pruner.Wait(), jc.ErrorIsNil)
	})
	fakeTimer.(*Timer).Fire()
	c.Assert(passedMaxLogs, gc.Equals, 3)
	// Reset will have been called with the actual PruneInterval
	c.Assert(fakeTimer.(*Timer).period, gc.Equals, coretesting.ShortWait)
}
