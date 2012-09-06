package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
	"sort"
	"time"
)

type MachineSuite struct {
	ConnSuite
	machine *state.Machine
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestMachineSetAgentAlive(c *C) {
	alive := s.machine.AgentAlive()
	c.Assert(alive, Equals, false)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Stop()

	s.State.ForcePresenceRefresh()
	alive = s.machine.AgentAlive()
	c.Assert(alive, Equals, true)
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *C) {
	// test -gocheck.f TestMachineWaitAgentAlive
	timeout := 5 * time.Second
	alive := s.machine.AgentAlive()
	c.Assert(alive, Equals, false)

	s.State.ForcePresenceRefresh()
	err := s.machine.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of machine 0: still not alive after timeout`)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)

	s.State.ForcePresenceRefresh()
	err = s.machine.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	err = pinger.Kill()
	c.Assert(err, IsNil)

	s.State.ForcePresenceRefresh()
	alive = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func (s *MachineSuite) TestMachineInstanceId(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", "spaceship/0"}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, _ := machine.InstanceId()
	c.Assert(iid, Equals, "spaceship/0")
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NotFoundError{})
	c.Assert(iid, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NotFoundError{})
	c.Assert(iid, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", ""}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NotFoundError{})
	c.Assert(iid, Equals, "")
}

func (s *MachineSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	n, err := s.machines.Find(D{{"instanceid", "umbrella/0"}}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
}

func (s *MachineSuite) TestMachineRefresh(c *C) {
	m0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	oldId, _ := m0.InstanceId()

	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, IsNil)
	err = m0.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)
	newId, _ := m0.InstanceId()

	m1Id, _ := m1.InstanceId()
	c.Assert(m1Id, Equals, oldId)
	err = m1.Refresh()
	c.Assert(err, IsNil)
	m1Id, _ = m1.InstanceId()
	c.Assert(m1Id, Equals, newId)
}

func (s *MachineSuite) TestMachineUnits(c *C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m3, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0, err := s.State.AddService("s0", dummy)
	c.Assert(err, IsNil)
	s1, err := s.State.AddService("s1", dummy)
	c.Assert(err, IsNil)
	s2, err := s.State.AddService("s2", dummy)
	c.Assert(err, IsNil)
	s3, err := s.State.AddService("s3", logging)
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
