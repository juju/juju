// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type AddUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-service-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"some-service-name", "--to", "bigglesplop"},
		err:  `invalid --to parameter "bigglesplop"`,
	}, {
		args: []string{"some-service-name", "-n", "2", "--to", "123"},
		err:  `cannot use --num-units > 1 with --to`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *gc.C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := testing.InitCommand(envcmd.Wrap(&AddUnitCommand{}), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func runAddUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&AddUnitCommand{}), args...)
	return err
}

func (s *AddUnitSuite) setupService(c *gc.C) *charm.URL {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
	return curl
}

func (s *AddUnitSuite) TestAddUnit(c *gc.C) {
	curl := s.setupService(c)

	err := runAddUnit(c, "some-service-name")
	c.Assert(err, gc.IsNil)
	s.AssertService(c, "some-service-name", curl, 2, 0)

	err = runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, gc.IsNil)
	s.AssertService(c, "some-service-name", curl, 4, 0)
}

// assertForceMachine ensures that the result of assigning a unit with --to
// is as expected.
func (s *AddUnitSuite) assertForceMachine(c *gc.C, svc *state.Service, expectedNumMachines, unitNum int, machineId string) {
	units, err := svc.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, expectedNumMachines)
	mid, err := units[unitNum].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(mid, gc.Equals, machineId)
}

func (s *AddUnitSuite) TestForceMachine(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	machine2, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = runAddUnit(c, "some-service-name", "--to", machine2.Id())
	c.Assert(err, gc.IsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, gc.IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine2.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineExistingContainer(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)

	err = runAddUnit(c, "some-service-name", "--to", container.Id())
	c.Assert(err, gc.IsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, gc.IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, container.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *gc.C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = runAddUnit(c, "some-service-name", "--to", "lxc:"+machine.Id())
	c.Assert(err, gc.IsNil)
	err = runAddUnit(c, "some-service-name", "--to", machine.Id())
	c.Assert(err, gc.IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine.Id()+"/lxc/0")
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}
