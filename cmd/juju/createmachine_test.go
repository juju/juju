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

type CreateMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&CreateMachineSuite{})

func runCreateMachine(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &CreateMachineCommand{}, args)
	return err
}

func (s *CreateMachineSuite) TestCreateMachine(c *C) {
	err := runCreateMachine(c)
	c.Assert(err, IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Alive)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	expectedCons := constraints.Value{}
	c.Assert(mcons, DeepEquals, expectedCons)
}

func (s *CreateMachineSuite) TestCreateMachineWithConstraints(c *C) {
	err := runCreateMachine(c, "--constraints", "mem=4G")
	c.Assert(err, IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Alive)
	mcons, err := m.Constraints()
	c.Assert(err, IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, DeepEquals, expectedCons)
}
