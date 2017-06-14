// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/workertest"
)

type statePoolSuite struct {
	statetesting.StateSuite
	State1, State2                    *state.State
	Pool                              *state.StatePool
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

	s.Pool = state.NewStatePool(s.State)
	s.AddCleanup(func(*gc.C) { s.Pool.Close() })
}

func (s *statePoolSuite) TestGet(c *gc.C) {
	st1, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1.ModelUUID(), gc.Equals, s.ModelUUID1)

	st2, _, err := s.Pool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2.ModelUUID(), gc.Equals, s.ModelUUID2)

	// Check that the same instances are returned
	// when a State for the same model is re-requested.
	st1_, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1_, gc.Equals, st1)

	st2_, _, err := s.Pool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2_, gc.Equals, st2)
}

func (s *statePoolSuite) TestGetWithControllerModel(c *gc.C) {
	// When a State for the controller model is requested, the same
	// State that was original passed in should be returned.
	st0, _, err := s.Pool.Get(s.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st0, gc.Equals, s.State)
}

func (s *statePoolSuite) TestGetSystemState(c *gc.C) {
	st0 := s.Pool.SystemState()
	c.Assert(st0, gc.Equals, s.State)
}

func (s *statePoolSuite) TestKillWorkers(c *gc.C) {
	// Get some State instances via the pool and extract their
	// internal workers.
	st1, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	w1 := state.GetInternalWorkers(st1)
	workertest.CheckAlive(c, w1)

	st2, _, err := s.Pool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	w2 := state.GetInternalWorkers(st2)
	workertest.CheckAlive(c, w2)

	// Now kill their workers.
	s.Pool.KillWorkers()

	// Ensure the internal workers for each State died.
	c.Check(workertest.CheckKilled(c, w1), jc.ErrorIsNil)
	c.Check(workertest.CheckKilled(c, w2), jc.ErrorIsNil)
}

func (s *statePoolSuite) TestClose(c *gc.C) {
	// Get some State instances.
	st1, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	st2, _, err := s.Pool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)

	// Now close them.
	err = s.Pool.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Confirm that controller State isn't closed.
	_, err = s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that new ones are returned if further States are
	// requested.
	st1_, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1_, gc.Not(gc.Equals), st1)

	st2_, _, err := s.Pool.Get(s.ModelUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2_, gc.Not(gc.Equals), st2)
}

func (s *statePoolSuite) TestTooManyReleases(c *gc.C) {
	st, firstRelease, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	// Get a second reference to the same model
	_, secondRelease, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	// Try to call the first releaser twice.
	firstRelease()
	firstRelease()

	removed, err := s.Pool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)

	// Not closed because r2 has not been called.
	assertNotClosed(c, st)

	removed = secondRelease()
	c.Assert(removed, jc.IsTrue)
	assertClosed(c, st)
}

func (s *statePoolSuite) TestReleaseOnSystemStateUUID(c *gc.C) {
	st, releaser, err := s.Pool.Get(s.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	removed := releaser()
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st)
}

func (s *statePoolSuite) TestRemoveSystemStateUUID(c *gc.C) {
	removed, err := s.Pool.Remove(s.ModelUUID)
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
	st, releaser, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)

	// Confirm the state isn't closed.
	removed := releaser()
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st)

	removed, err = s.Pool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsTrue)

	assertClosed(c, st)
}

func (s *statePoolSuite) TestRemoveWithRefsClosesOnLastRelease(c *gc.C) {
	st, firstRelease, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, secondRelease, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	// Now there are two references to the state.
	// Sanity check!
	assertNotClosed(c, st)

	// Doesn't close while there are refs still held.
	removed, err := s.Pool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st)

	removed = firstRelease()
	// Hasn't been closed - still one outstanding reference.
	c.Assert(removed, jc.IsFalse)
	assertNotClosed(c, st)

	// Should be closed when it's released back into the pool.
	removed = secondRelease()
	c.Assert(removed, jc.IsTrue)
	assertClosed(c, st)
}

func (s *statePoolSuite) TestGetRemovedNotAllowed(c *gc.C) {
	_, _, err := s.Pool.Get(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Pool.Remove(s.ModelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.Pool.Get(s.ModelUUID1)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("model %v has been removed", s.ModelUUID1))
}
