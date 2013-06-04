// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
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

func (s *AddMachineSuite) _assertAddContainer(c *C, parentId, containerId string, container state.ContainerType) {
	m, err := s.State.Machine(parentId)
	c.Assert(err, IsNil)
	c.Assert(m.NumChildren(), Equals, 1)
	m, err = s.State.Machine(containerId)
	c.Assert(err, IsNil)
	c.Assert(m.NumChildren(), Equals, 0)
	c.Assert(m.ContainerType(), Equals, container)
}

func (s *AddMachineSuite) TestAddContainerToNewMachine(c *C) {
	for i, container := range state.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("/%s", container))
		c.Assert(err, IsNil)
		s._assertAddContainer(c, "0", fmt.Sprintf("0/%s/%d", container, i), container)
	}
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *C) {
	err := runAddMachine(c)
	c.Assert(err, IsNil)
	err = runAddMachine(c)
	c.Assert(err, IsNil)
	for i, container := range state.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("1/%s", container))
		c.Assert(err, IsNil)
		s._assertAddContainer(c, "1", fmt.Sprintf("1/%s/%d", container, i), container)
	}
}

func (s *AddMachineSuite) TestAddMachineErrors(c *C) {
	err := runAddMachine(c, ":foo")
	c.Assert(err, ErrorMatches, `malformed container argument ":foo"`)
	err = runAddMachine(c, "--container-constraints", "mem=4G")
	c.Assert(err, ErrorMatches, `container constraints not applicable when no container is specified`)
	err = runAddMachine(c, "0:/lxc", "--constraints", "mem=4G")
	c.Assert(err, ErrorMatches, `machine constraints not applicable when parent machine is specified`)
	err = runAddMachine(c, "/lxc", "--constraints", "mem=4G", "--container-constraints", "mem=8G")
	c.Assert(err, ErrorMatches, `container constraints "mem=8192M" not compatible with machine constraints "mem=4096M"`)
}
