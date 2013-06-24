// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type MinimumUnitsSuite struct {
	ConnSuite
	service *state.Service
}

var _ = Suite(&MinimumUnitsSuite{})

func (s *MinimumUnitsSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.service, err = s.State.AddService("dummy-service", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
}

func (s *MinimumUnitsSuite) assertRevno(c *C, expectedRevno int, expectedErr error) {
	revno, err := state.MinimumUnitsRevno(s.State, s.service.Name())
	c.Assert(err, Equals, expectedErr)
	c.Assert(revno, Equals, expectedRevno)
}

var setMinimumUnitsTests = []struct {
	about   string
	initial int
	changes []int
	revno   int
	err     error
}{
	{
		// Revno is set to zero on creation.
		about:   "setting minimum units",
		changes: []int{42},
	},
	{
		// Revno is increased by the update operation.
		about:   "updating minimum units",
		initial: 1,
		changes: []int{42},
		revno:   1,
	},
	{
		// Revno does not change.
		about:   "updating minimum units with the same value",
		initial: 42,
		changes: []int{42},
	},
	{
		// Revno is increased by each update.
		about:   "increasing minimum units multiple times",
		initial: 1,
		changes: []int{2, 3, 4},
		revno:   3,
	},
	{
		// Revno does not change.
		about:   "decreasing minimum units multiple times",
		initial: 5,
		changes: []int{3, 2, 1},
	},
	{
		// No-op.
		about:   "removing not existent minimum units",
		changes: []int{0},
		err:     mgo.ErrNotFound,
	},
	{
		// The document is deleted.
		about:   "removing existing minimum units",
		initial: 42,
		changes: []int{0},
		err:     mgo.ErrNotFound,
	},
}

func (s *MinimumUnitsSuite) TestSetMinimumUnits(c *C) {
	var err error
	service := s.service
	for i, t := range setMinimumUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		// Set up initial minimum units if required.
		if t.initial > 0 {
			err = service.SetMinimumUnits(t.initial)
			c.Assert(err, IsNil)
		}
		// Insert/update minimum units.
		for _, input := range t.changes {
			err = service.SetMinimumUnits(input)
			c.Assert(err, IsNil)
			c.Assert(service.MinimumUnits(), Equals, input)
		}
		// Check the document existence and revno.
		s.assertRevno(c, t.revno, t.err)
		// Clean up, if required, the minimumUnits document.
		err = service.SetMinimumUnits(0)
		c.Assert(err, IsNil)
	}
}

func (s *MinimumUnitsSuite) TestInvalidMinimumUnits(c *C) {
	err := s.service.SetMinimumUnits(-1)
	c.Assert(err, ErrorMatches, `.* minimum units must be a positive number`)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsInsertRetry(c *C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.service.SetMinimumUnits(41)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, nil)
	}, func() {
		s.assertRevno(c, 1, nil)
	}).Check()
	err := s.service.SetMinimumUnits(42)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinimumUnits(), Equals, 42)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsUpdateRetry(c *C) {
	err := s.service.SetMinimumUnits(41)
	c.Assert(err, IsNil)
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.service.SetMinimumUnits(0)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}, func() {
		s.assertRevno(c, 0, nil)
	}).Check()
	err = s.service.SetMinimumUnits(42)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinimumUnits(), Equals, 42)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsRemoveBefore(c *C) {
	err := s.service.SetMinimumUnits(41)
	c.Assert(err, IsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.service.SetMinimumUnits(0)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}).Check()
	err = s.service.SetMinimumUnits(0)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinimumUnits(), Equals, 0)
}

func (s *MinimumUnitsSuite) testDestroyServiceBefore(c *C, initial, input int) {
	err := s.service.SetMinimumUnits(initial)
	c.Assert(err, IsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.service.Destroy()
		c.Assert(err, IsNil)
	}).Check()
	err = s.service.SetMinimumUnits(input)
	c.Assert(err, ErrorMatches, `.* service is no longer alive`)
	s.assertRevno(c, 0, mgo.ErrNotFound)
	c.Assert(s.service.Life(), Not(Equals), state.Alive)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsInsertDestroyServiceBefore(c *C) {
	s.testDestroyServiceBefore(c, 0, 42)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsUpdateDestroyServiceBefore(c *C) {
	s.testDestroyServiceBefore(c, 1, 42)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsRemoveDestroyServiceBefore(c *C) {
	s.testDestroyServiceBefore(c, 1, 0)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsRequiredByService(c *C) {
	err := s.service.SetMinimumUnits(1)
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, nil)
	// Add a unit to the service.
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	// Destroy the unit.
	err = unit.Destroy()
	c.Assert(err, IsNil)
	// Ensure the unit properly advanced its state and
	// the revno has been increased.
	c.Assert(unit.Life(), Not(Equals), state.Alive)
	s.assertRevno(c, 1, nil)
	// Destroy the service.
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	// Ensure the service properly advanced its state and
	// the document has been removed.
	c.Assert(s.service.Life(), Not(Equals), state.Alive)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func (s *MinimumUnitsSuite) TestMinimumUnitsNotRequiredByService(c *C) {
	// Add a unit to the service.
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	// Destroy the unit.
	err = unit.Destroy()
	c.Assert(err, IsNil)
	// Ensure the unit properly advanced its state and
	// the minimum units document has not been created.
	c.Assert(unit.Life(), Not(Equals), state.Alive)
	s.assertRevno(c, 0, mgo.ErrNotFound)
	// Destroy the service.
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	// Ensure the service properly advanced its state and
	// the minimum units document is still missing.
	c.Assert(s.service.Life(), Not(Equals), state.Alive)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}
