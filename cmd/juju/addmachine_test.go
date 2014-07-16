// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/testing"
)

type AddMachineSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddMachineSuite{})

func runAddMachine(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&AddMachineCommand{}), args...)
}

func (s *AddMachineSuite) TestAddMachine(c *gc.C) {
	context, err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
	c.Assert(m.Series(), gc.DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *AddMachineSuite) TestSSHPlacement(c *gc.C) {
	s.PatchValue(&manualProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "42", nil
	})
	context, err := runAddMachine(c, "ssh:10.1.2.3")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 42\n")
}

func (s *AddMachineSuite) TestSSHPlacementError(c *gc.C) {
	s.PatchValue(&manualProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "", fmt.Errorf("failed to initialize warp core")
	})
	context, err := runAddMachine(c, "ssh:10.1.2.3")
	c.Assert(err, gc.ErrorMatches, "failed to initialize warp core")
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func (s *AddMachineSuite) TestAddMachineWithSeries(c *gc.C) {
	context, err := runAddMachine(c, "--series", "series")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *AddMachineSuite) TestAddMachineWithConstraints(c *gc.C) {
	context, err := runAddMachine(c, "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *AddMachineSuite) TestAddTwoMachinesWithConstraints(c *gc.C) {
	context, err := runAddMachine(c, "--constraints", "mem=4G", "-n", "2")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\ncreated machine 1\n")
	for i := 0; i < 2; i++ {
		m, err := s.State.Machine(strconv.Itoa(i))
		c.Assert(err, gc.IsNil)
		mcons, err := m.Constraints()
		c.Assert(err, gc.IsNil)
		expectedCons := constraints.MustParse("mem=4G")
		c.Assert(mcons, gc.DeepEquals, expectedCons)
	}
}

func (s *AddMachineSuite) TestAddTwoMachinesWithContainers(c *gc.C) {
	context, err := runAddMachine(c, "lxc", "-n", "2")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created container 0/lxc/0\ncreated container 1/lxc/0\n")
	for i := 0; i < 2; i++ {
		machine := fmt.Sprintf("%d/%s/0", i, instance.LXC)
		s._assertAddContainer(c, strconv.Itoa(i), machine, instance.LXC)
	}
}

func (s *AddMachineSuite) TestAddTwoMachinesWithContainerDirective(c *gc.C) {
	_, err := runAddMachine(c, "lxc:1", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "cannot use -n when specifying a placement directive")
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
		context, err := runAddMachine(c, string(ctype))
		c.Assert(err, gc.IsNil)
		machine := fmt.Sprintf("%d/%s/0", i, ctype)
		c.Assert(testing.Stderr(context), gc.Equals, "created container "+machine+"\n")
		s._assertAddContainer(c, strconv.Itoa(i), machine, ctype)
	}
}

func (s *AddMachineSuite) TestAddContainerToExistingMachine(c *gc.C) {
	context, err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")
	for i, container := range instance.ContainerTypes {
		machineNum := strconv.Itoa(i + 1)
		context, err = runAddMachine(c)
		c.Assert(err, gc.IsNil)
		c.Assert(testing.Stderr(context), gc.Equals, "created machine "+machineNum+"\n")
		context, err := runAddMachine(c, fmt.Sprintf("%s:%s", container, machineNum))
		c.Assert(err, gc.IsNil)
		machine := fmt.Sprintf("%s/%s/0", machineNum, container)
		c.Assert(testing.Stderr(context), gc.Equals, "created container "+machine+"\n")
		s._assertAddContainer(c, machineNum, machine, container)
	}
}

func (s *AddMachineSuite) TestAddUnsupportedContainerToMachine(c *gc.C) {
	context, err := runAddMachine(c)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	m.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	context, err = runAddMachine(c, "lxc:0")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxc containers")
	c.Assert(testing.Stderr(context), gc.Equals, "failed to create 1 machine\n")
}

func (s *AddMachineSuite) TestAddMachineErrors(c *gc.C) {
	_, err := runAddMachine(c, ":lxc")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: :lxc placement is invalid`)
	_, err = runAddMachine(c, "lxc:")
	c.Check(err, gc.ErrorMatches, `invalid value "" for "lxc" scope: expected machine-id`)
	_, err = runAddMachine(c, "2")
	c.Check(err, gc.ErrorMatches, `machine-id cannot be specified when adding machines`)
	_, err = runAddMachine(c, "foo")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: foo placement is invalid`)
	_, err = runAddMachine(c, "foo:bar")
	c.Check(err, gc.ErrorMatches, `invalid environment name "foo"`)
	_, err = runAddMachine(c, "dummyenv:invalid")
	c.Check(err, gc.ErrorMatches, `cannot add a new machine: invalid placement is invalid`)
	_, err = runAddMachine(c, "lxc", "--constraints", "container=lxc")
	c.Check(err, gc.ErrorMatches, `container constraint "lxc" not allowed when adding a machine`)
}

func (s *AddMachineSuite) TestAddThreeMachinesWithTwoFailures(c *gc.C) {
	fakeApi := fakeAddMachineAPI{}
	s.PatchValue(&getAddMachineAPI, func(c *AddMachineCommand) (addMachineAPI, error) {
		return &fakeApi, nil
	})
	fakeApi.successOrder = []bool{true, false, false}
	expectedOutput := `created machine 0
failed to create 2 machines
`
	context, err := runAddMachine(c, "-n", "3")
	c.Assert(err, gc.ErrorMatches, "something went wrong, something went wrong")
	c.Assert(testing.Stderr(context), gc.Equals, expectedOutput)
}

type fakeAddMachineAPI struct {
	successOrder []bool
	currentOp    int
}

func (f *fakeAddMachineAPI) Close() error {
	return nil
}

func (f *fakeAddMachineAPI) EnvironmentUUID() string {
	return "fake-uuid"
}

func (f *fakeAddMachineAPI) AddMachines(args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	results := []params.AddMachinesResult{}
	for i := range args {
		if f.successOrder[i] {
			results = append(results, params.AddMachinesResult{
				Machine: strconv.Itoa(i),
				Error:   nil,
			})
		} else {
			results = append(results, params.AddMachinesResult{
				Machine: string(i),
				Error:   &params.Error{"something went wrong", "1"},
			})
		}
		f.currentOp++
	}
	return results, nil
}

func (f *fakeAddMachineAPI) DestroyMachines(machines ...string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeAddMachineAPI) ProvisioningScript(params.ProvisioningScriptParams) (script string, err error) {
	return "", fmt.Errorf("not implemented")
}
