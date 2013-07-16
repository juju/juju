// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type AddUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-service-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"some-service-name", "--force-machine", "bigglesplop"},
		err:  `invalid --force-machine parameter "bigglesplop"`,
	}, {
		args: []string{"some-service-name", "-n", "2", "--force-machine", "123"},
		err:  `cannot use --num-units > 1 with --force-machine`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := testing.InitCommand(&AddUnitCommand{}, t.args)
		c.Check(err, ErrorMatches, t.err)
	}
}

func runAddUnit(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &AddUnitCommand{}, args)
	return err
}

func (s *AddUnitSuite) setupService(c *C) *charm.URL {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
	return curl
}

func (s *AddUnitSuite) TestAddUnit(c *C) {
	curl := s.setupService(c)

	err := runAddUnit(c, "some-service-name")
	c.Assert(err, IsNil)
	s.AssertService(c, "some-service-name", curl, 2, 0)

	err = runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, IsNil)
	s.AssertService(c, "some-service-name", curl, 4, 0)
}

// assertForceMachine ensures that the result of assigning a unit with --force-machine
// is as expected.
func (s *AddUnitSuite) assertForceMachine(c *C, svc *state.Service, expectedNumMachines, unitNum int, machineId string) {
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, expectedNumMachines)
	mid, err := units[unitNum].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, machineId)
}

func (s *AddUnitSuite) TestForceMachine(c *C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	machine2, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)

	err = runAddUnit(c, "some-service-name", "--force-machine", machine2.Id())
	c.Assert(err, IsNil)
	err = runAddUnit(c, "some-service-name", "--force-machine", machine.Id())
	c.Assert(err, IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine2.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineExistingContainer(c *C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	params := &state.AddMachineParams{
		ParentId:      machine.Id(),
		Series:        "precise",
		ContainerType: instance.LXC,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(params)
	c.Assert(err, IsNil)

	err = runAddUnit(c, "some-service-name", "--force-machine", container.Id())
	c.Assert(err, IsNil)
	err = runAddUnit(c, "some-service-name", "--force-machine", machine.Id())
	c.Assert(err, IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, container.Id())
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *C) {
	curl := s.setupService(c)
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)

	err = runAddUnit(c, "some-service-name", "--force-machine", "lxc:"+machine.Id())
	c.Assert(err, IsNil)
	err = runAddUnit(c, "some-service-name", "--force-machine", machine.Id())
	c.Assert(err, IsNil)
	svc, _ := s.AssertService(c, "some-service-name", curl, 3, 0)
	s.assertForceMachine(c, svc, 3, 1, machine.Id()+"/lxc/0")
	s.assertForceMachine(c, svc, 3, 2, machine.Id())
}
