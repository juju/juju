// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"strconv"
)

type AddMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddMachineSuite{})

func runAddMachine(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, &AddMachineCommand{}, args)
	return err
}

func (s *AddMachineSuite) TestAddMachine(c *gc.C) {
	err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
	c.Assert(m.Series(), gc.DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	expectedCons := constraints.Value{}
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *AddMachineSuite) TestAddMachineWithSeries(c *gc.C) {
	err := runAddMachine(c, "--series", "series")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *AddMachineSuite) TestAddMachineWithConstraints(c *gc.C) {
	err := runAddMachine(c, "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *AddMachineSuite) _assertAddContainer(c *gc.C, parentId, containerId string, ctype instance.ContainerType) {
	m, err := s.State.Machine(parentId)
	c.Assert(err, gc.IsNil)
	containers, err := m.Containers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.DeepEquals, []string{containerId})
	container, err := s.State.Machine(containerId)
	c.Assert(err, gc.IsNil)
	containers, err = container.Containers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.DeepEquals, []string(nil))
	c.Assert(container.ContainerType(), gc.Equals, ctype)
}

func (s *AddMachineSuite) TestAddContainerToNewMachine(c *gc.C) {
	for i, ctype := range instance.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("%s", ctype))
		c.Assert(err, gc.IsNil)
		s._assertAddContainer(c, strconv.Itoa(2*i), fmt.Sprintf("0/%s/0", ctype), ctype)
	}
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *gc.C) {
	err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	err = runAddMachine(c)
	c.Assert(err, gc.IsNil)
	for i, container := range instance.SupportedContainerTypes {
		err := runAddMachine(c, fmt.Sprintf("%s:1", container))
		c.Assert(err, gc.IsNil)
		s._assertAddContainer(c, "1", fmt.Sprintf("1/%s/%d", container, i), container)
	}
}

func (s *AddMachineSuite) TestAddMachineErrors(c *gc.C) {
	err := runAddMachine(c, ":lxc")
	c.Assert(err, gc.ErrorMatches, `malformed container argument ":lxc"`)
	err = runAddMachine(c, "lxc:")
	c.Assert(err, gc.ErrorMatches, `malformed container argument "lxc:"`)
	err = runAddMachine(c, "2")
	c.Assert(err, gc.ErrorMatches, `malformed container argument "2"`)
	err = runAddMachine(c, "foo")
	c.Assert(err, gc.ErrorMatches, `malformed container argument "foo"`)
	err = runAddMachine(c, "lxc", "--constraints", "container=lxc")
	c.Assert(err, gc.ErrorMatches, `container constraint "lxc" not allowed when adding a machine`)
}

func (s *AddMachineSuite) TestAddMachineSanityCheckConstraints(c *gc.C) {
	dummy.SanityCheckConstraintsError = errors.New("computer says no")
	err := runAddMachine(c, "lxc:0")
	c.Assert(err, gc.ErrorMatches, "computer says no")
}
