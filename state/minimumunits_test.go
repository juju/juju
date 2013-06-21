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

var minimumUnitsTests = []struct {
	about   string
	initial int
	input   int
	revno   int
	err     error
}{
	{
		about: "test setting minimum units",
		input: 42,
	},
	{
		about:   "test updating minimum units",
		initial: 1,
		input:   42,
		revno:   1,
	},
	{
		about: "test removing unexistent minimum units",
		input: 0,
		err:   mgo.ErrNotFound,
	},
	{
		about:   "test removing existing minimum units",
		initial: 42,
		input:   0,
		err:     mgo.ErrNotFound,
	},
}

func (s *MinimumUnitsSuite) TestSetMinimumUnits(c *C) {
	var err error
	service := s.service
	for i, t := range minimumUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		// Set up initial minimum units if required.
		if t.initial > 0 {
			err = service.SetMinimumUnits(t.initial)
			c.Assert(err, IsNil)
		}
		// Insert/update minimum units.
		err = service.SetMinimumUnits(t.input)
		c.Assert(err, IsNil)
		c.Assert(service.MinimumUnits(), Equals, t.input)
		// Check the document existence and revno.
		revno, err := state.MinimumUnitsRevno(s.State, service.Name())
		c.Assert(err, Equals, t.err)
		c.Assert(revno, Equals, t.revno)
		// Clean up the existing document.
		err = service.SetMinimumUnits(0)
		c.Assert(err, IsNil)
	}
}

func (s *UnitSuite) TestDestroyServiceRetry(c *C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.service.SetMinimumUnits(1)
		c.Assert(err, IsNil)
	}, func() {
		c.Assert(s.service.MinimumUnits(), Equals, 42)
	}).Check()
	err := s.service.SetMinimumUnits(42)
	c.Assert(err, IsNil)
}
