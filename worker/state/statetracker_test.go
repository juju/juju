// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	workerstate "github.com/juju/juju/worker/state"
)

type StateTrackerSuite struct {
	statetesting.StateSuite
	stateTracker workerstate.StateTracker
}

var _ = gc.Suite(&StateTrackerSuite{})

func (s *StateTrackerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.stateTracker = workerstate.NewStateTracker(s.StatePool)
}

func (s *StateTrackerSuite) TestDoneWithNoUse(c *gc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestTooManyDones(c *gc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertStatePoolClosed(c, s.StatePool)

	err = s.stateTracker.Done()
	c.Assert(err, gc.Equals, workerstate.ErrStateClosed)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestUse(c *gc.C) {
	pool, systemState, err := s.stateTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(systemState, gc.Equals, s.State)
	c.Check(err, jc.ErrorIsNil)

	systemState, err = pool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(systemState, gc.Equals, s.State)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StateTrackerSuite) TestUseAndDone(c *gc.C) {
	// Ref count starts at 1 (the creator/owner)

	_, _, err := s.stateTracker.Use()
	// 2
	c.Check(err, jc.ErrorIsNil)

	_, _, err = s.stateTracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 2
	assertStatePoolNotClosed(c, s.StatePool)

	_, _, err = s.stateTracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 2
	assertStatePoolNotClosed(c, s.StatePool)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 1
	assertStatePoolNotClosed(c, s.StatePool)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 0
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestUseWhenClosed(c *gc.C) {
	c.Assert(s.stateTracker.Done(), jc.ErrorIsNil)

	pool, _, err := s.stateTracker.Use()
	c.Check(pool, gc.IsNil)
	c.Check(err, gc.Equals, workerstate.ErrStateClosed)
}

func assertStatePoolNotClosed(c *gc.C, pool *state.StatePool) {
	systemState, err := pool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(systemState, gc.NotNil)
	err = systemState.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func assertStatePoolClosed(c *gc.C, pool *state.StatePool) {
	systemState, err := pool.SystemState()
	c.Assert(err, gc.ErrorMatches, "pool is closed")
	c.Assert(systemState, gc.IsNil)
}

func isTxnLogStarted(report map[string]interface{}) bool {
	// Sometimes when we call pool.Report() not all the workers have started yet, so we check
	next := report
	var ok bool
	for _, p := range []string{"txn-watcher", "workers", "txnlog"} {
		if child, ok := next[p]; !ok {
			return false
		} else {
			next = child.(map[string]interface{})
		}
	}
	state, ok := next["state"]
	return ok && state == "started"
}

func (s *StateTrackerSuite) TestReport(c *gc.C) {
	pool, _, err := s.stateTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	start := time.Now()
	report := pool.Report()
	for !isTxnLogStarted(report) {
		if time.Since(start) >= testing.LongWait {
			c.Fatalf("txlog worker did not start after %v", testing.LongWait)
		}
		time.Sleep(time.Millisecond)
		report = pool.Report()
	}
	c.Check(report, gc.NotNil)
	// We don't have any State models in the pool, but we do have the
	// txn-watcher report and the system state.
	c.Check(report, gc.HasLen, 3)
	c.Check(report["pool-size"], gc.Equals, 0)

	// Calling Report increments the request count in the system
	// state's hubwatcher stats, so zero that out before comparing.
	removeRequestCount := func(report map[string]interface{}) map[string]interface{} {
		next := report
		for _, p := range []string{"system", "workers", "txnlog", "report"} {
			child, ok := next[p]
			if !ok {
				c.Fatalf("couldn't find system.workers.txnlog.report")
			}
			next = child.(map[string]interface{})
		}
		delete(next, "request-count")
		return report
	}
	report = removeRequestCount(report)
	c.Check(removeRequestCount(s.stateTracker.Report()), gc.DeepEquals, report)
	c.Check(removeRequestCount(s.stateTracker.Report()), gc.DeepEquals, report)
	c.Check(removeRequestCount(s.stateTracker.Report()), gc.DeepEquals, report)
}
