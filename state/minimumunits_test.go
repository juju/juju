// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type MinUnitsSuite struct {
	ConnSuite
	service *state.Service
}

var _ = Suite(&MinUnitsSuite{})

func (s *MinUnitsSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.service, err = s.State.AddService("dummy-service", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
}

func (s *MinUnitsSuite) assertRevno(c *C, expectedRevno int, expectedErr error) {
	revno, err := state.MinUnitsRevno(s.State, s.service.Name())
	c.Assert(err, Equals, expectedErr)
	c.Assert(revno, Equals, expectedRevno)
}

func (s *MinUnitsSuite) TestSetMinUnits(c *C) {
	service := s.service
	for i, t := range []struct {
		about   string
		initial int
		changes []int
		revno   int
		err     error
	}{{
		// Revno is set to zero on creation.
		about:   "setting minimum units",
		changes: []int{42},
	}, {
		// Revno is increased by the update operation.
		about:   "updating minimum units",
		initial: 1,
		changes: []int{42},
		revno:   1,
	}, {
		// Revno does not change.
		about:   "updating minimum units with the same value",
		initial: 42,
		changes: []int{42},
	}, {
		// Revno is increased by each update.
		about:   "increasing minimum units multiple times",
		initial: 1,
		changes: []int{2, 3, 4},
		revno:   3,
	}, {
		// Revno does not change.
		about:   "decreasing minimum units multiple times",
		initial: 5,
		changes: []int{3, 2, 1},
	}, {
		// No-op.
		about:   "removing not existent minimum units",
		changes: []int{0},
		err:     mgo.ErrNotFound,
	}, {
		// The document is deleted.
		about:   "removing existing minimum units",
		initial: 42,
		changes: []int{0},
		err:     mgo.ErrNotFound,
	}} {
		c.Logf("test %d. %s", i, t.about)
		// Set up initial minimum units if required.
		if t.initial > 0 {
			err := service.SetMinUnits(t.initial)
			c.Assert(err, IsNil)
		}
		// Insert/update minimum units.
		for _, input := range t.changes {
			err := service.SetMinUnits(input)
			c.Assert(err, IsNil)
			c.Assert(service.MinUnits(), Equals, input)
			c.Assert(service.Refresh(), IsNil)
			c.Assert(service.MinUnits(), Equals, input)
		}
		// Check the document existence and revno.
		s.assertRevno(c, t.revno, t.err)
		// Clean up, if required, the minUnits document.
		err := service.SetMinUnits(0)
		c.Assert(err, IsNil)
	}
}

func (s *MinUnitsSuite) TestInvalidMinUnits(c *C) {
	err := s.service.SetMinUnits(-1)
	c.Assert(err, ErrorMatches, `cannot set minimum units for service "dummy-service": cannot set a negative minimum number of units`)
}

func (s *MinUnitsSuite) TestMinUnitsInsertRetry(c *C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.service.SetMinUnits(41)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, nil)
	}, func() {
		s.assertRevno(c, 1, nil)
	}).Check()
	err := s.service.SetMinUnits(42)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinUnits(), Equals, 42)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateRetry(c *C) {
	err := s.service.SetMinUnits(41)
	c.Assert(err, IsNil)
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.service.SetMinUnits(0)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}, func() {
		s.assertRevno(c, 0, nil)
	}).Check()
	err = s.service.SetMinUnits(42)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinUnits(), Equals, 42)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveBefore(c *C) {
	err := s.service.SetMinUnits(41)
	c.Assert(err, IsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.service.SetMinUnits(0)
		c.Assert(err, IsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}).Check()
	err = s.service.SetMinUnits(0)
	c.Assert(err, IsNil)
	c.Assert(s.service.MinUnits(), Equals, 0)
}

func (s *MinUnitsSuite) testDestroyOrRemoveServiceBefore(c *C, initial, input int, preventRemoval bool) {
	err := s.service.SetMinUnits(initial)
	c.Assert(err, IsNil)
	expectedErr := `cannot set minimum units for service "dummy-service": service "dummy-service" not found`
	if preventRemoval {
		expectedErr = `cannot set minimum units for service "dummy-service": service is no longer alive`
		_, err = s.service.AddUnit()
		c.Assert(err, IsNil)
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.service.Destroy()
		c.Assert(err, IsNil)
	}).Check()
	err = s.service.SetMinUnits(input)
	c.Assert(err, ErrorMatches, expectedErr)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func (s *MinUnitsSuite) TestMinUnitsInsertDestroyServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 0, 42, true)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateDestroyServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 1, 42, true)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveDestroyServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 1, 0, true)
}

func (s *MinUnitsSuite) TestMinUnitsInsertRemoveServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 0, 42, false)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateRemoveServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 1, 42, false)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveRemoveServiceBefore(c *C) {
	s.testDestroyOrRemoveServiceBefore(c, 1, 0, false)
}

func (s *MinUnitsSuite) TestMinUnitsSetDestroyEntities(c *C) {
	err := s.service.SetMinUnits(1)
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, nil)

	// Add two units to the service for later use.
	unit1, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	unit2, err := s.service.AddUnit()
	c.Assert(err, IsNil)

	// Destroy a unit and ensure the revno has been increased.
	preventUnitDestroyRemove(c, unit1)
	err = unit1.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 1, nil)

	// Remove a unit and ensure the revno has been increased..
	err = unit2.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 2, nil)

	// Destroy the service and ensure the minUnits document has been removed.
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func (s *MinUnitsSuite) TestMinUnitsNotSetDestroyEntities(c *C) {
	// Add two units to the service for later use.
	unit1, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	unit2, err := s.service.AddUnit()
	c.Assert(err, IsNil)

	// Destroy a unit and ensure the minUnits document has not been created.
	preventUnitDestroyRemove(c, unit1)
	err = unit1.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)

	// Remove a unit and ensure the minUnits document has not been created.
	err = unit2.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)

	// Destroy the service and ensure the minUnits document is still missing.
	err = s.service.Destroy()
	c.Assert(err, IsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}
