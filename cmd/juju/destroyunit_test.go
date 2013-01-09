package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
)

type DestroyUnitSuite struct {
	repoSuite
}

var _ = Suite(&DestroyUnitSuite{})

func runDestroyUnit(c *C, args ...string) error {
	com := &DestroyUnitCommand{}
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *DestroyUnitSuite) TestErrors(c *C) {
	err := runDestroyUnit(c)
	c.Assert(err, ErrorMatches, "no units specified")
	err = runDestroyUnit(c, "not-a-unit")
	c.Assert(err, ErrorMatches, `invalid unit name: "not-a-unit"`)
}

func (s *DestroyUnitSuite) TestDestroyPrincipalUnits(c *C) {
	// Create 3 principal units.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	for i := 0; i < 3; i++ {
		_, err = wordpress.AddUnit()
		c.Assert(err, IsNil)
	}

	// Destroy 2 of them; check they become Dying.
	err = runDestroyUnit(c, "wordpress/0", "wordpress/1")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "wordpress/1", state.Dying)

	// Try to destroy the remaining one along with a pre-destroyed one; check
	// it fails.
	err = runDestroyUnit(c, "wordpress/2", "wordpress/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "wordpress/0" is not alive`)
	s.assertUnitLife(c, "wordpress/2", state.Alive)

	// Try to destroy the remaining one along with a nonexistent one; check it
	// fails.
	err = runDestroyUnit(c, "wordpress/2", "boojum/123")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "boojum/123" is not alive`)
	s.assertUnitLife(c, "wordpress/2", state.Alive)

	// Destroy the remaining unit on its own, accidentally specifying it twice;
	// this should work.
	err = runDestroyUnit(c, "wordpress/2", "wordpress/2")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/2", state.Dying)
}

func (s *DestroyUnitSuite) TestDestroySubordinateUnits(c *C) {
	// Create a principal and a subordinate.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = wordpress0.SetPrivateAddress("meh")
	c.Assert(err, IsNil)
	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, IsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = runDestroyUnit(c, "logging/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Try to destroy the principal and the subordinate together; check it fails.
	err = runDestroyUnit(c, "wordpress/0", "logging/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "wordpress/0", state.Alive)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Destroy the principal; check the subordinate does not become Dying. (This
	// is the unit agent's responsibility.)
	err = runDestroyUnit(c, "wordpress/0")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "logging/0", state.Alive)
}

func (s *DestroyUnitSuite) assertUnitLife(c *C, name string, life state.Life) {
	unit, err := s.State.Unit(name)
	c.Assert(err, IsNil)
	c.Assert(unit.Refresh(), IsNil)
	c.Assert(unit.Life(), Equals, life)
}
