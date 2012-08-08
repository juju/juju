package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
)

type UnitSuite struct {
	ConnSuite
	charm *state.Charm
	unit  *state.Unit
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	address, err := s.unit.PublicAddress()
	c.Assert(err, ErrorMatches, `public address of unit "wordpress/0" not found`)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = s.unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	address, err := s.unit.PrivateAddress()
	c.Assert(err, ErrorMatches, `private address of unit "wordpress/0" not found`)
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = s.unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	setting, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": flag already set`)
	retry, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(retry, Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	setting, err = s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.SetResolved(state.ResolvedMode(999))
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: 999`)
}
