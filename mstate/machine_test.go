package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
	"launchpad.net/juju-core/version"
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
	alive, err := s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Stop()

	s.State.Sync()
	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *C) {
	// test -gocheck.f TestMachineWaitAgentAlive
	timeout := 5 * time.Second
	alive, err := s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	s.State.StartSync()
	err = s.machine.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of machine 0: still not alive after timeout`)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)

	s.State.StartSync()
	err = s.machine.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	err = pinger.Kill()
	c.Assert(err, IsNil)

	s.State.Sync()
	alive, err = s.machine.AgentAlive()
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

func (s *MachineSuite) TestRefreshWhenNotAlive(c *C) {
	// Refresh should work regardless of liveness status.
	m := s.machine
	err := s.SetInstanceId("foo")
	c.Assert(err, IsNil)

	err = m.Kill()
	c.Assert(err, IsNil)
	err = m.Refresh()
	c.Assert(err, IsNil)
	id, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "foo")

	err = m.Die()
	c.Assert(err, IsNil)
	err = m.Refresh()
	c.Assert(err, IsNil)

	id, err = m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "foo")
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

type machineInfo struct {
	tools      *state.Tools
	instanceId string
}

func tools(tools int, url string) *state.Tools {
	return &state.Tools{
		URL: url,
		Binary: version.Binary{
			Number: version.Number{0, 0, tools},
			Series: "series",
			Arch:   "arch",
		},
	}
}

var watchMachineTests = []struct {
	test func(m *state.Machine) error
	want machineInfo
}{
	{
		func(m *state.Machine) error {
			return nil
		},
		machineInfo{
			tools: &state.Tools{},
		},
	},
	{
		func(m *state.Machine) error {
			return m.SetInstanceId("m-foo")
		},
		machineInfo{
			tools:      &state.Tools{},
			instanceId: "m-foo",
		},
	},
	{
		func(m *state.Machine) error {
			return m.SetInstanceId("")
		},
		machineInfo{
			tools:      &state.Tools{},
			instanceId: "",
		},
	},
	// TODO SetAgentTools is missing.
	//{
	//	func(m *state.Machine) error {
	//		return m.SetAgentTools(tools(3, "baz"))
	//	},
	//	machineInfo{
	//		tools: tools(3, "baz"),
	//	},
	//},
	//{
	//	func(m *state.Machine) error {
	//		return m.SetAgentTools(tools(4, "khroomph"))
	//	},
	//	machineInfo{
	//		tools: tools(4, "khroomph"),
	//	},
	//},
}

func (s *MachineSuite) TestWatchMachine(c *C) {
	w := s.machine.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	for i, test := range watchMachineTests {
		c.Logf("test %d", i)
		err := test.test(s.machine)
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case m, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(m.Id(), Equals, s.machine.Id())
			var info machineInfo
			// TODO AgentTools is missing.
			info.tools = test.want.tools
			//info.tools, err = m.AgentTools()
			//c.Assert(err, IsNil)
			info.instanceId, err = m.InstanceId()
			if _, ok := err.(*state.NotFoundError); !ok {
				c.Assert(err, IsNil)
			}
			c.Assert(info, DeepEquals, test.want)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v", test.want)
		}
	}
	select {
	case got := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
