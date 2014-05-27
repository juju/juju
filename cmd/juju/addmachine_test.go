// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
  "fmt"
  "strconv"

  jc "github.com/juju/testing/checkers"
  gc "launchpad.net/gocheck"

  "launchpad.net/juju-core/cmd"
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

func runAddMachine(c *gc.C, args ...string) (*cmd.Context, error) {
  cxt, err := testing.RunCommand(c, envcmd.Wrap(&AddMachineCommand{}), args...)
  return cxt, err
}

func (s *AddMachineSuite) TestAddMachine(c *gc.C) {
  cxt, err := runAddMachine(c)
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-0\n")
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
  cxt, err := runAddMachine(c, "--series", "series")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-0\n")
  c.Assert(err, gc.IsNil)
  m, err := s.State.Machine("0")
  c.Assert(err, gc.IsNil)
  c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *AddMachineSuite) TestAddMachineWithConstraints(c *gc.C) {
  cxt, err := runAddMachine(c, "--constraints", "mem=4G")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-0\n")
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
    cxt, err := runAddMachine(c, string(ctype))
    c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created container machine-"+strconv.Itoa(i)+"-"+string(ctype)+"-0\n")
    c.Assert(err, gc.IsNil)
    s._assertAddContainer(c, strconv.Itoa(i), fmt.Sprintf("%d/%s/0", i, ctype), ctype)
  }
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *gc.C) {
  cxt, err := runAddMachine(c)
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-0\n")
  c.Assert(err, gc.IsNil)
  for i, container := range instance.ContainerTypes {
    machineNum := strconv.Itoa(i + 1)
    cxt, err = runAddMachine(c)
    c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-"+machineNum+"\n")
    c.Assert(err, gc.IsNil)
    cxt, err := runAddMachine(c, fmt.Sprintf("%s:%s", container, machineNum))
    if string(container) == "machine" {
      c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created container machine-"+machineNum+"\n")
    } else {
      c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created container machine-"+machineNum+"-"+string(container)+"-0\n")
    }
    c.Assert(err, gc.IsNil)
    s._assertAddContainer(c, machineNum, fmt.Sprintf("%s/%s/0", machineNum, container), container)
  }
}

func (s *AddMachineSuite) TestAddUnsupportedContainerToMachine(c *gc.C) {
  cxt, err := runAddMachine(c)
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "created machine machine-0\n")
  c.Assert(err, gc.IsNil)
  m, err := s.State.Machine("0")
  c.Assert(err, gc.IsNil)
  m.SetSupportedContainers([]instance.ContainerType{instance.KVM})
  cxt, err = runAddMachine(c, "lxc:0")
  c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxc containers")
}

func (s *AddMachineSuite) TestAddMachineErrors(c *gc.C) {
  cxt, err := runAddMachine(c, ":lxc")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `cannot add a new machine: :lxc placement is invalid`)
  cxt, err = runAddMachine(c, "lxc:")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `invalid value "" for "lxc" scope: expected machine-id`)
  cxt, err = runAddMachine(c, "2")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `machine-id cannot be specified when adding machines`)
  cxt, err = runAddMachine(c, "foo")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `cannot add a new machine: foo placement is invalid`)
  cxt, err = runAddMachine(c, "foo:bar")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `invalid environment name "foo"`)
  cxt, err = runAddMachine(c, "dummyenv:invalid")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `cannot add a new machine: invalid placement is invalid`)
  cxt, err = runAddMachine(c, "lxc", "--constraints", "container=lxc")
  c.Assert(testing.Stderr(cxt), gc.DeepEquals, "")
  c.Check(err, gc.ErrorMatches, `container constraint "lxc" not allowed when adding a machine`)
}
