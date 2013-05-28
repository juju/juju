// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type AddMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&AddMachineSuite{})

func runAddMachine(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &AddMachineCommand{}, args)
	return err
}

func (s *AddMachineSuite) TestAddMachine(c *C) {
	err := runAddMachine(c)
	c.Assert(err, IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Alive)
	c.Assert(m.Series(), DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	expectedCons := constraints.Value{}
	c.Assert(mcons, DeepEquals, expectedCons)
}

func (s *AddMachineSuite) TestAddMachineWithSeries(c *C) {
	err := runAddMachine(c, "--series", "series")
	c.Assert(err, IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Series(), DeepEquals, "series")
}

func (s *AddMachineSuite) TestAddMachineWithConstraints(c *C) {
	err := runAddMachine(c, "--constraints", "mem=4G")
	c.Assert(err, IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, DeepEquals, expectedCons)
}
