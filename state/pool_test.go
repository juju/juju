// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type statePoolSuite struct {
	statetesting.StateSuite
	State1, State2                    *state.State
	ModelUUID, ModelUUID1, ModelUUID2 string
}

var _ = gc.Suite(&statePoolSuite{})

func (s *statePoolSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.ModelUUID = s.State.ModelUUID()

	s.State1 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.State1.Close() })
	s.ModelUUID1 = s.State1.ModelUUID()

	s.State2 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })
	s.ModelUUID2 = s.State2.ModelUUID()
}

func (s *statePoolSuite) TestGet(c *gc.C) {
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1.ModelUUID(), gc.Equals, s.ModelUUID1)

	st2, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2.ModelUUID(), gc.Equals, s.ModelUUID2)

	// Check that the same instances are returned
	// when a State for the same model is re-requested.
	st1_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1_.State, gc.Equals, st1.State)

	st2_, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2_.State, gc.Equals, st2.State)
}

func (s *statePoolSuite) TestGetWithControllerModel(c *gc.C) {
	// When a State for the controller model is requested, the same
	// State that was original passed in should be returned.
	st0, err := s.StatePool.Get(s.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st0.State, gc.Equals, s.State)
}

func (s *statePoolSuite) TestGetSystemState(c *gc.C) {
	st0 := s.StatePool.SystemState()
	c.Assert(st0, gc.Equals, s.State)
}

func (s *statePoolSuite) TestClose(c *gc.C) {
	// Get some State instances.
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	st2, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)

	// Now close them.
	err = s.StatePool.Close()
	c.Assert(err, jc.ErrorIsNil)

	assertStateClosed := func(st *state.State) {
		c.Assert(func() { st.Ping() }, gc.PanicMatches, "Session already closed")
	}

	assertStateClosed(s.State)
	assertStateClosed(st1.State)
	assertStateClosed(st2.State)

	// Requests to Get and GetModel now return errors.
	st1_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, gc.ErrorMatches, "pool is closed")
	c.Assert(st1_, gc.IsNil)

	st2_, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, gc.ErrorMatches, "pool is closed")
	c.Assert(st2_, gc.IsNil)
}

func (s *statePoolSuite) TestTooManyReleases(c *gc.C) {
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	// Get a second reference to the same model
	st2, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	// Try to call the first releaser twice.
	st1.Release()
	st1.Release()

	removed, err := s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)

	// Not closed because r2 has not been called.
	assertNotClosed(c, st1.State)

	removed = st2.Release()
	c.Assert(removed, jc.IsTrue)
	assertClosed(c, st1.State)
}

func (s *statePoolSuite) TestReleaseOnSystemStateUUID(c *gc.C) {
	st, err := s.StatePool.Get(s.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	removed := st.Release()
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st.State)
}

func (s *statePoolSuite) TestRemoveSystemStateUUID(c *gc.C) {
	removed, err := s.StatePool.Remove(s.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, s.State)
}

func assertNotClosed(c *gc.C, st *state.State) {
	_, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func assertClosed(c *gc.C, st *state.State) {
	w := state.GetInternalWorkers(st)
	c.Check(workertest.CheckKilled(c, w), jc.ErrorIsNil)
}

func (s *statePoolSuite) TestRemoveWithNoRefsCloses(c *gc.C) {
	st, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	// Confirm the state isn't closed.
	removed := st.Release()
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st.State)

	removed, err = s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsTrue)

	assertClosed(c, st.State)
}

func (s *statePoolSuite) TestRemoveWithRefsClosesOnLastRelease(c *gc.C) {
	st, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	st2, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	// Now there are two references to the state.
	// Sanity check!
	assertNotClosed(c, st.State)

	// Doesn't close while there are refs still held.
	removed, err := s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st.State)

	removed = st.Release()
	// Hasn't been closed - still one outstanding reference.
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st.State)

	// Should be closed when it's released back into the pool.
	removed = st2.Release()
	c.Assert(removed, jc.IsTrue)
	assertClosed(c, st.State)
}

func (s *statePoolSuite) TestGetRemovedNotAllowed(c *gc.C) {
	_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("model %v has been removed", s.ModelUUID1))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *statePoolSuite) TestReport(c *gc.C) {
	report := s.StatePool.Report()
	c.Check(report, gc.HasLen, 3)
}
