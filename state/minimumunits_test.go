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

func (s *MinimumUnitsSuite) TestMinimumUnitsSetDestroyEntities(c *C) {
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

func (s *MinimumUnitsSuite) TestMinimumUnitsNotSetDestroyEntities(c *C) {
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

func assertAliveUnits(c *C, service *state.Service, expected int) {
	count, err := service.AliveUnitsCount()
	c.Assert(err, IsNil)
	c.Assert(count, Equals, expected)
}

func (s *MinimumUnitsSuite) TestAliveUnitsCount(c *C) {
	assertAliveUnits(c, s.service, 0)
	// Add three units to the service.
	for i := 0; i < 3; i++ {
		_, err := s.service.AddUnit()
		c.Assert(err, IsNil)
	}
	// Now there are three alive units.
	assertAliveUnits(c, s.service, 3)
	// Destroy a unit, preventing it from being removed.
	allUnits, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(allUnits, HasLen, 3)
	preventUnitDestroyRemove(c, allUnits[0])
	err = allUnits[0].Destroy()
	c.Assert(err, IsNil)
	// Ensure the number of alive units changed.
	assertAliveUnits(c, s.service, 2)
}

var ensureMinimumUnitsTests = []struct {
	about    string
	initial  int
	minimum  int
	destroy  int
	expected int
}{
	{
		about: "no minimum units set",
	},
	{
		about:    "initial units > minimum units",
		initial:  2,
		minimum:  1,
		expected: 2,
	},
	{
		about:    "initial units == minimum units",
		initial:  2,
		minimum:  2,
		expected: 2,
	},
	{
		about:    "initial units < minimum units",
		initial:  1,
		minimum:  2,
		expected: 2,
	},
	{
		about:    "alive units < minimum units",
		initial:  2,
		minimum:  2,
		destroy:  1,
		expected: 2,
	},
	{
		about:    "add multiple units",
		initial:  6,
		minimum:  5,
		destroy:  4,
		expected: 5,
	},
}

func (s *MinimumUnitsSuite) TestEnsureMinimumUnits(c *C) {
	var err error
	service := s.service
	for i, t := range ensureMinimumUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		// Set up initial units if required.
		for i := 0; i < t.initial; i++ {
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		}
		// Set up minimum units if required.
		err = service.SetMinimumUnits(t.minimum)
		c.Assert(err, IsNil)
		// Destroy units if required.
		allUnits, err := service.AllUnits()
		c.Assert(err, IsNil)
		for i := 0; i < t.destroy; i++ {
			preventUnitDestroyRemove(c, allUnits[i])
			err = allUnits[i].Destroy()
			c.Assert(err, IsNil)
		}
		// Ensure the minimum amount of units is correctly restored.
		err = service.EnsureMinimumUnits()
		c.Assert(err, IsNil)
		assertAliveUnits(c, service, t.expected)
		// Clean up the minimumUnits document and the units.
		err = service.SetMinimumUnits(0)
		c.Assert(err, IsNil)
		err = s.State.Cleanup()
		c.Assert(err, IsNil)
		allUnits, err = service.AllUnits()
		c.Assert(err, IsNil)
		for _, unit := range allUnits {
			err = unit.Destroy()
			c.Assert(err, IsNil)
		}
	}
}

func (s *MinimumUnitsSuite) TestEnsureMinimumUnitsNotAlive(c *C) {
	err := s.service.SetMinimumUnits(2)
	c.Assert(err, IsNil)
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	preventUnitDestroyRemove(c, unit)
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	err = s.service.EnsureMinimumUnits()
	c.Assert(err, ErrorMatches, `.* cannot add unit to service .* not alive`)
}

func (s *MinimumUnitsSuite) TestEnsureMinimumUnitsNotFound(c *C) {
	err := s.service.SetMinimumUnits(2)
	c.Assert(err, IsNil)
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	err = s.service.EnsureMinimumUnits()
	c.Assert(err, ErrorMatches, `.* cannot add unit to service .* not found`)
}
