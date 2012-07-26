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
	machine *state.Machine
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) Config(c *C) *state.ConfigNode {
	config, err := state.ReadConfigNode(s.State, fmt.Sprintf("/machines/machine-%010d", s.machine.Id()))
	c.Assert(err, IsNil)
	return config
}

func (s *MachineSuite) TestMachineInstanceId(c *C) {
	config := s.Config(c)
	config.Set("provider-machine-id", "spaceship/0")
	_, err := config.Write()
	c.Assert(err, IsNil)

	id, err := s.machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "spaceship/0")
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *C) {
	config := s.Config(c)
	config.Set("provider-machine-id", map[int]int{})
	_, err := config.Write()
	c.Assert(err, IsNil)

	id, err := s.machine.InstanceId()
	c.Assert(err.Error(), Equals, `invalid type for value "" of instance id of machine 0: string`)
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *C) {
	id, err := s.machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NoInstanceIdError{})
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *C) {
	config := s.Config(c)
	config.Set("provider-machine-id", "")
	_, err := config.Write()
	c.Assert(err, IsNil)

	id, err := s.machine.InstanceId()
	c.Assert(err, FitsTypeOf, &state.NoInstanceIdError{})
	c.Assert(id, Equals, "")
}

func (s *MachineSuite) TestMachineSetInstanceId(c *C) {
	err := s.machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)
	config := s.Config(c)
	c.Assert(config.Map(), DeepEquals, map[string]interface{}{"provider-machine-id": "umbrella/0"})
}

func (s *MachineSuite) TestMachineSetAgentAlive(c *C) {
	alive, err := s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	alive, err := s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = s.machine.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of machine 0: presence: still not alive after timeout`)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = s.machine.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
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

var watchMachineUnitsTests = []struct {
	test func(m *state.Machine, units []*state.Unit) error
	want func(units []*state.Unit) *state.MachineUnitsChange
}{
	{
		func(_ *state.Machine, _ []*state.Unit) error {
			return nil
		},
		func(_ []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[0].AssignToMachine(m)
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Added: []*state.Unit{units[0], units[1]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[2].AssignToMachine(m)
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Added: []*state.Unit{units[2]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[0].UnassignFromMachine()
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Removed: []*state.Unit{units[0], units[1]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[2].UnassignFromMachine()
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Removed: []*state.Unit{units[2]}}
		},
	},
}

func (s *MachineSuite) TestWatchMachineUnits(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)

	units := make([]*state.Unit, 3)
	units[0], err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	units[1], err = logging.AddUnitSubordinateTo(units[0])
	c.Assert(err, IsNil)
	units[2], err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	unitsWatcher := s.machine.WatchUnits()
	defer func() {
		c.Assert(unitsWatcher.Stop(), IsNil)
	}()

	for i, test := range watchMachineUnitsTests {
		c.Logf("test %d", i)
		err := test.test(s.machine, units)
		c.Assert(err, IsNil)
		want := test.want(units)
		select {
		case got, ok := <-unitsWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(unitNames(got.Added), DeepEquals, unitNames(want.Added))
			c.Assert(unitNames(got.Removed), DeepEquals, unitNames(want.Removed))
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v", want)
		}
	}

	select {
	case got := <-unitsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func unitNames(units []*state.Unit) (s []string) {
	for _, u := range units {
		s = append(s, u.Name())
	}
	return
}
