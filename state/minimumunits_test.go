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

func (s *MinimumUnitsSuite) TestMinimumUnits(c *C) {
	var err error
	for i, t := range minimumUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		if t.initial > 0 {
			err = s.service.SetMinimumUnits(t.initial)
			c.Assert(err, IsNil)
		}
		err = s.service.SetMinimumUnits(t.input)
		c.Assert(err, IsNil)
		c.Assert(s.service.MinimumUnits(), Equals, t.input)
		revno, err := state.MinimumUnitsRevno(s.State, s.service.Name())
		c.Assert(err, Equals, t.err)
		c.Assert(revno, Equals, t.revno)
	}
}
