// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"strconv"
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

func (s *AddMachineSuite) _assertAddContainer(c *C, parentId, containerId string, ctype instance.ContainerType) {
	m, err := s.State.Machine(parentId)
	c.Assert(err, IsNil)
	containers, err := m.Containers()
	c.Assert(err, IsNil)
	c.Assert(containers, DeepEquals, []string{containerId})
	container, err := s.State.Machine(containerId)
	c.Assert(err, IsNil)
	containers, err = container.Containers()
	c.Assert(err, IsNil)
	c.Assert(containers, DeepEquals, []string(nil))
	c.Assert(container.ContainerType(), Equals, ctype)
}

func (s *AddMachineSuite) TestAddContainerToNewMachine(c *C) {
	for i, ctype := range instance.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("%s", ctype))
		c.Assert(err, IsNil)
		s._assertAddContainer(c, strconv.Itoa(2*i), fmt.Sprintf("0/%s/0", ctype), ctype)
	}
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *C) {
	err := runAddMachine(c)
	c.Assert(err, IsNil)
	err = runAddMachine(c)
	c.Assert(err, IsNil)
	for i, container := range instance.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("%s:1", container))
		c.Assert(err, IsNil)
		s._assertAddContainer(c, "1", fmt.Sprintf("1/%s/%d", container, i), container)
	}
}

func (s *AddMachineSuite) TestAddMachineErrors(c *C) {
	err := runAddMachine(c, ":foo")
	c.Assert(err, ErrorMatches, `malformed container argument ":foo"`)
	err = runAddMachine(c, "foo:")
	c.Assert(err, ErrorMatches, `malformed container argument "foo:"`)
	err = runAddMachine(c, "lxc", "--constraints", "container=lxc")
	c.Assert(err, ErrorMatches, `container constraint "lxc" not allowed when adding a machine`)
}
