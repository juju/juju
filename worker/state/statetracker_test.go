// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	workerstate "github.com/juju/juju/worker/state"
)

type StateTrackerSuite struct {
	statetesting.StateSuite
	pool         *state.State
	stateTracker workerstate.StateTracker
}

var _ = gc.Suite(&StateTrackerSuite{})

func (s *StateTrackerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	// Close the state pool, as it's not needed, and it
	// refers to the state object's mongo session. If we
	// do not close the pool, its embedded watcher may
	// attempt to access mongo after it has been closed
	// by the state tracker.
	err := s.StatePool.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.stateTracker = workerstate.NewStateTracker(s.State)
}

func (s *StateTrackerSuite) TearDownTest(c *gc.C) {
	// Even though we no longer have to worry about the StateSuite's
	// StatePool, we do have to make sure the one in the stateTracker
	// is closed.

	for {
		err := s.stateTracker.Done()
		if err == workerstate.ErrStateClosed {
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}

	s.StateSuite.TearDownTest(c)
}

func (s *StateTrackerSuite) TestDoneWithNoUse(c *gc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertStateClosed(c, s.State)
}

func (s *StateTrackerSuite) TestTooManyDones(c *gc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, jc.ErrorIsNil)
	assertStateClosed(c, s.State)

	err = s.stateTracker.Done()
	c.Assert(err, gc.Equals, workerstate.ErrStateClosed)
	assertStateClosed(c, s.State)
}

func (s *StateTrackerSuite) TestUse(c *gc.C) {
	pool, err := s.stateTracker.Use()
	c.Check(pool.SystemState(), gc.Equals, s.State)
	c.Check(err, jc.ErrorIsNil)

	pool, err = s.stateTracker.Use()
	c.Check(pool.SystemState(), gc.Equals, s.State)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StateTrackerSuite) TestUseAndDone(c *gc.C) {
	// Ref count starts at 1 (the creator/owner)

	_, err := s.stateTracker.Use()
	// 2
	c.Check(err, jc.ErrorIsNil)

	_, err = s.stateTracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 2
	assertStateNotClosed(c, s.State)

	_, err = s.stateTracker.Use()
	// 3
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 2
	assertStateNotClosed(c, s.State)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 1
	assertStateNotClosed(c, s.State)

	c.Check(s.stateTracker.Done(), jc.ErrorIsNil)
	// 0
	assertStateClosed(c, s.State)
}

func (s *StateTrackerSuite) TestUseWhenClosed(c *gc.C) {
	c.Assert(s.stateTracker.Done(), jc.ErrorIsNil)

	pool, err := s.stateTracker.Use()
	c.Check(pool, gc.IsNil)
	c.Check(err, gc.Equals, workerstate.ErrStateClosed)
}

func assertStateNotClosed(c *gc.C, st *state.State) {
	err := st.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func assertStateClosed(c *gc.C, st *state.State) {
	c.Assert(st.Ping, gc.PanicMatches, "Session already closed")
}

func (s *StateTrackerSuite) TestReport(c *gc.C) {
	pool, err := s.stateTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	report := pool.Report()
	c.Check(report, gc.NotNil)
	// We don't have any State models in the pool, but we do have the txn-watcher report.
	c.Check(report, gc.HasLen, 2)
	c.Check(report["pool-size"], gc.Equals, 2)
	c.Check(s.stateTracker.Report(), gc.DeepEquals, report)
}
