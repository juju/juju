package mstate_test

import (
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
	"sort"
)

type MachineSuite struct {
	UtilSuite
	machine *state.Machine
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *C) {
	s.UtilSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TearDownTest(c *C) {
	s.UtilSuite.TearDownTest(c)
}

func (s *MachineSuite) TestMachineInstanceId(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(
		bson.D{{"_id", machine.Id()}},
		bson.D{{"$set", bson.D{{"instanceid", "spaceship/0"}}}},
	)
	c.Assert(err, IsNil)

	iid, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(iid, Equals, "spaceship/0")
}

func (s *MachineSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	n, err := s.machines.Find(bson.D{{"instanceid", "umbrella/0"}}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
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