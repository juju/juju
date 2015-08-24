// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/statushistorypruner"
)

type mockTimer struct {
	period time.Duration
	c      chan time.Time
}

func (t *mockTimer) Reset(d time.Duration) bool {
	t.period = d
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

func newMockTimer(d time.Duration) worker.PeriodicTimer {
	return &mockTimer{period: d,
		c: make(chan time.Time),
	}
}

var _ = gc.Suite(&statusHistoryPrunerSuite{})

type statusHistoryPrunerSuite struct {
	coretesting.BaseSuite
}

func (s *statusHistoryPrunerSuite) TestWorker(c *gc.C) {
	var passedMaxLogs int
	fakePruner := func(_ *state.State, maxLogs int) error {
		passedMaxLogs = maxLogs
		return nil
	}
	params := statushistorypruner.HistoryPrunerParams{
		MaxLogsPerState: 3,
		PruneInterval:   coretesting.ShortWait,
	}
	fakeTimer := newMockTimer(coretesting.LongWait)

	fakeTimerFunc := func(d time.Duration) worker.PeriodicTimer {
		// construction of timer should be with 0 because we intend it to
		// run once before waiting.
		c.Assert(d, gc.Equals, 0*time.Nanosecond)
		return fakeTimer
	}
	pruner := statushistorypruner.NewPruneWorker(
		&state.State{},
		&params,
		fakeTimerFunc,
		fakePruner,
	)
	s.AddCleanup(func(*gc.C) {
		pruner.Kill()
		c.Assert(pruner.Wait(), jc.ErrorIsNil)
	})
	err := fakeTimer.(*mockTimer).fire()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(passedMaxLogs, gc.Equals, 3)
	// Reset will have been called with the actual PruneInterval
	c.Assert(fakeTimer.(*mockTimer).period, gc.Equals, coretesting.ShortWait)
}
