// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type AddMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddMachineSuite{})

func runAddMachine(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&AddMachineCommand{}), args)
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
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
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
	for i, ctype := range instance.ContainerTypes {
		c.Logf("test %d: %s", i, ctype)
		err := runAddMachine(c, string(ctype))
		c.Assert(err, gc.IsNil)
		s._assertAddContainer(c, strconv.Itoa(i), fmt.Sprintf("%d/%s/0", i, ctype), ctype)
	}
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *gc.C) {
	err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	for i, container := range instance.ContainerTypes {
		machineNum := strconv.Itoa(i + 1)
		err = runAddMachine(c)
		c.Assert(err, gc.IsNil)
		err := runAddMachine(c, fmt.Sprintf("%s:%s", container, machineNum))
		c.Assert(err, gc.IsNil)
		s._assertAddContainer(c, machineNum, fmt.Sprintf("%s/%s/0", machineNum, container), container)
	}
}

func (s *AddMachineSuite) TestAddUnsupportedContainerToMachine(c *gc.C) {
	err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	m.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	err = runAddMachine(c, "lxc:0")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxc containers")
}

func (s *AddMachineSuite) TestAddMachineErrors(c *gc.C) {
	err := runAddMachine(c, ":lxc")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: :lxc placement is invalid`)
	err = runAddMachine(c, "lxc:")
	c.Check(err, gc.ErrorMatches, `invalid value "" for "lxc" scope: expected machine-id`)
	err = runAddMachine(c, "2")
	c.Check(err, gc.ErrorMatches, `machine-id cannot be specified when adding machines`)
	err = runAddMachine(c, "foo")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: foo placement is invalid`)
	err = runAddMachine(c, "foo:bar")
	c.Check(err, gc.ErrorMatches, `invalid environment name "foo"`)
	err = runAddMachine(c, "dummyenv:invalid")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: invalid placement is invalid`)
	err = runAddMachine(c, "lxc", "--constraints", "container=lxc")
	c.Check(err, gc.ErrorMatches, `container constraint "lxc" not allowed when adding a machine`)
}
