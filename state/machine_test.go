package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"sort"
	"time"
)

type MachineSuite struct {
	ConnSuite
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestAddMachine(c *C) {
	machine0, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000000", "machine-0000000001"})
}

func (s *MachineSuite) TestRemoveMachine(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	err = s.St.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000001"})

	// Removing a non-existing machine has to fail.
	err = s.St.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "can't remove machine 0: machine not found")
}

func (s *MachineSuite) TestMachineInstanceId(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.St, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", "spaceship/0")
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "spaceship/0")
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.St, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", map[int]int{})
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err.Error(), Equals, "invalid internal machine id type map[interface {}]interface {} for machine 0")
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NoInstanceIdError{})
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.St, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", "")
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NoInstanceIdError{})
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	actual, err := state.ReadConfigNode(s.St, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	c.Assert(actual.Map(), DeepEquals, map[string]interface{}{"provider-machine-id": "umbrella/0"})
}

func (s *MachineSuite) TestReadMachine(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.St.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *MachineSuite) TestReadNonExistentMachine(c *C) {
	_, err := s.St.Machine(0)
	c.Assert(err, ErrorMatches, "machine 0 not found")

	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.St.Machine(1)
	c.Assert(err, ErrorMatches, "machine 1 not found")
}

func (s *MachineSuite) TestAllMachines(c *C) {
	s.AssertMachineCount(c, 0)
	_, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	s.AssertMachineCount(c, 1)
	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	s.AssertMachineCount(c, 2)
}

func (s *MachineSuite) TestMachineSetAgentAlive(c *C) {
	machine0, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)

	alive, err := machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := machine0.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	machine0, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)

	alive, err := machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = machine0.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of machine 0: presence: still not alive after timeout`)

	pinger, err := machine0.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = machine0.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func (s *MachineSuite) TestMachineUnits(c *C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	m3, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	dummy := s.Charm(c, "dummy")
	logging := s.Charm(c, "logging")
	s0, err := s.St.AddService("s0", dummy)
	c.Assert(err, IsNil)
	s1, err := s.St.AddService("s1", dummy)
	c.Assert(err, IsNil)
	s2, err := s.St.AddService("s2", dummy)
	c.Assert(err, IsNil)
	s3, err := s.St.AddService("s3", logging)
	c.Assert(err, IsNil)

	units := make([][]*state.Unit, 4)
	for i, svc := range []*state.Service{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = svc.AddUnit()
			c.Assert(err, IsNil)
		}
	}
	// Add the logging units subordinate to the s2 units.
	units[3] = make([]*state.Unit, 3)
	for i := range units[3] {
		units[3][i], err = s3.AddUnitSubordinateTo(units[2][i])
	}

	assignments := []struct {
		machine      *state.Machine
		units        []*state.Unit
		subordinates []*state.Unit
	}{
		{m1, []*state.Unit{units[0][0]}, nil},
		{m2, []*state.Unit{units[0][1], units[1][0], units[1][1], units[2][0]}, []*state.Unit{units[3][0]}},
		{m3, []*state.Unit{units[2][2]}, []*state.Unit{units[3][2]}},
	}

	for _, a := range assignments {
		for _, u := range a.units {
			err := u.AssignToMachine(a.machine)
			c.Assert(err, IsNil)
		}
	}

	for i, a := range assignments {
		c.Logf("test %d", i)
		got, err := a.machine.Units()
		c.Assert(err, IsNil)
		expect := sortedUnitNames(append(a.units, a.subordinates...))
		c.Assert(sortedUnitNames(got), DeepEquals, expect)
	}
}

func sortedUnitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	sort.Strings(names)
	return names
}
