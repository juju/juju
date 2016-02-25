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
	st           *state.State
	stateTracker workerstate.StateTracker
}

var _ = gc.Suite(&StateTrackerSuite{})

func (s *StateTrackerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.stateTracker = workerstate.NewStateTracker(s.State)
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
	st, err := s.stateTracker.Use()
	c.Check(st, gc.Equals, s.State)
	c.Check(err, jc.ErrorIsNil)

	st, err = s.stateTracker.Use()
	c.Check(st, gc.Equals, s.State)
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

	st, err := s.stateTracker.Use()
	c.Check(st, gc.IsNil)
	c.Check(err, gc.Equals, workerstate.ErrStateClosed)
}

func assertStateNotClosed(c *gc.C, st *state.State) {
	err := st.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func assertStateClosed(c *gc.C, st *state.State) {
	c.Assert(st.Ping, gc.PanicMatches, "Session already closed")
}
