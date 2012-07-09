package mstate_test

import (
	. "launchpad.net/gocheck"
//	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
//	"time"
)

type UnitSuite struct {
	UtilSuite
	charm *state.Charm
	unit  *state.Unit
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.UtilSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
}

