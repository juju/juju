package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
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
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestLifeJobManageEnviron(c *C) {
	// A JobManageEnviron machine must never advance lifecycle.
	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	err = m.Destroy()
	c.Assert(err, ErrorMatches, "machine 1 is required by the environment")
	err = m.EnsureDead()
	c.Assert(err, ErrorMatches, "machine 1 is required by the environment")
}

func (s *MachineSuite) TestLifeJobHostUnits(c *C) {
	// A machine with an assigned unit must not advance lifecycle.
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, IsNil)
	err = s.machine.Destroy()
	c.Assert(err, Equals, state.ErrHasAssignedUnits)
	err = s.machine.EnsureDead()
	c.Assert(err, Equals, state.ErrHasAssignedUnits)
	c.Assert(s.machine.Life(), Equals, state.Alive)

	// Once no unit is assigned, lifecycle can advance.
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	err = s.machine.Destroy()
	c.Assert(s.machine.Life(), Equals, state.Dying)
	c.Assert(err, IsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)

	// A machine that has never had units assigned can advance lifecycle.
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = m.Destroy()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Dying)
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Dead)
}

func (s *MachineSuite) TestRemove(c *C) {
	err := s.machine.Remove()
	c.Assert(err, ErrorMatches, "cannot remove machine 0: machine is not dead")
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestMachineSetAgentAlive(c *C) {
	alive, err := s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, NotNil)
	defer pinger.Stop()

	s.State.Sync()
	alive, err = s.machine.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *MachineSuite) TestEntityName(c *C) {
	c.Assert(s.machine.EntityName(), Equals, "machine-0")
}

func (s *MachineSuite) TestMachineEntityName(c *C) {
	c.Assert(state.MachineEntityName("10"), Equals, "machine-10")
}

func (s *MachineSuite) TestSetMongoPassword(c *C) {
	testSetMongoPassword(c, func(st *state.State) (entity, error) {
		return st.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestSetPassword(c *C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *C) {
	timeout := 200 * time.Millisecond
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
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", "spaceship/0"}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, _ := machine.InstanceId()
	c.Assert(iid, Equals, state.InstanceId("spaceship/0"))
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, ok := machine.InstanceId()
	c.Assert(ok, Equals, false)
	c.Assert(iid, Equals, state.InstanceId(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *C) {
	iid, ok := s.machine.InstanceId()
	c.Assert(ok, Equals, false)
	c.Assert(string(iid), Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", ""}}}},
	)
	c.Assert(err, IsNil)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	iid, ok := machine.InstanceId()
	c.Assert(ok, Equals, false)
	c.Assert(string(iid), Equals, "")
}

func (s *MachineSuite) TestMachineSetInstanceId(c *C) {
	err := s.machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, IsNil)
	id, ok := m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(string(id), Equals, "umbrella/0")
}

func (s *MachineSuite) TestMachineRefresh(c *C) {
	m0, err := s.State.AddMachine("series", state.JobHostUnits)
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

	err = m0.EnsureDead()
	c.Assert(err, IsNil)
	err = m0.Remove()
	c.Assert(err, IsNil)
	err = m0.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *MachineSuite) TestRefreshWhenNotAlive(c *C) {
	// Refresh should work regardless of liveness status.
	m := s.machine
	err := m.SetInstanceId("foo")
	c.Assert(err, IsNil)

	testWhenDying(c, s.machine, noErr, noErr, func() error {
		return m.Refresh()
	})
}

func (s *MachineSuite) TestMachinePrincipalUnits(c *C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	m3, err := s.State.AddMachine("series", state.JobHostUnits)
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
	eps, err := s.State.InferEndpoints([]string{"s2", "s3"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	for _, u := range units[2] {
		ru, err := rel.Unit(u)
		c.Assert(err, IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, IsNil)
	}
	units[3], err = s3.AllUnits()
	c.Assert(err, IsNil)

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
			Number: version.Number{
				Major: 0, Minor: 0, Patch: tools,
			},
			Series: "series",
			Arch:   "arch",
		},
	}
}

var watchMachineTests = []func(m *state.Machine) error{
	func(m *state.Machine) error {
		return nil
	},
	func(m *state.Machine) error {
		return m.SetInstanceId("m-foo")
	},
	func(m *state.Machine) error {
		return m.SetInstanceId("")
	},
	func(m *state.Machine) error {
		return m.SetAgentTools(tools(3, "baz"))
	},
}

func (s *MachineSuite) TestWatchMachine(c *C) {
	w := s.machine.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	for i, test := range watchMachineTests {
		c.Logf("test %d", i)
		err := test(s.machine)
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
		case <-time.After(5 * time.Second):
			c.Fatalf("did not get change")
		}
	}
	select {
	case got := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchPrincipalUnits()
	defer stop(c, w)
	assertNoChange := func() {
		s.State.Sync()
		select {
		case <-time.After(50 * time.Millisecond):
		case got, ok := <-w.Changes():
			c.Fatalf("unexpected change: %#v, %v", got, ok)
		}
	}
	assertChange := func(expect ...string) {
		s.State.Sync()
		select {
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("timed out")
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if len(expect) == 0 {
				c.Assert(got, HasLen, 0)
			} else {
				sort.Strings(expect)
				sort.Strings(got)
				c.Assert(expect, DeepEquals, got)
			}
		}
		assertNoChange()
	}
	assertChange()

	// Change machine; no change.
	err := s.machine.SetInstanceId("cheese")
	c.Assert(err, IsNil)

	// Assign a unit; change detected.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql0.AssignToMachine(s.machine)
	c.Assert(err, IsNil)
	assertChange("mysql/0")

	// Change the unit; no change.
	err = mysql0.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)
	assertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql1.AssignToMachine(s.machine)
	c.Assert(err, IsNil)
	err = mysql0.Destroy()
	c.Assert(err, IsNil)
	assertChange("mysql/0", "mysql/1")

	// Add a subordinate to the Alive unit; no change.
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, IsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, IsNil)
	logging0, err := logging.Unit("logging/0")
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)
	assertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, IsNil)
	assertChange("mysql/0")

	// Stop watcher; check Changes chan closed.
	assertClosed := func() {
		select {
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("not closed")
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, false)
		}
	}
	stop(c, w)
	assertClosed()

	// Start a fresh watcher; check both principals reported.
	w = s.machine.WatchPrincipalUnits()
	defer stop(c, w)
	assertChange("mysql/0", "mysql/1")

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, IsNil)
	assertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, IsNil)
	assertNoChange()

	// Unassign the unit; check change.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, IsNil)
	assertChange("mysql/1")
}

func (s *MachineSuite) TestWatchUnits(c *C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchUnits()
	defer stop(c, w)
	assertNoChange := func() {
		s.State.Sync()
		select {
		case <-time.After(50 * time.Millisecond):
		case got, ok := <-w.Changes():
			c.Fatalf("unexpected change: %#v, %v", got, ok)
		}
	}
	assertChange := func(expect ...string) {
		s.State.Sync()
		select {
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("timed out")
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if len(expect) == 0 {
				c.Assert(got, HasLen, 0)
			} else {
				sort.Strings(expect)
				sort.Strings(got)
				c.Assert(expect, DeepEquals, got)
			}
		}
		assertNoChange()
	}
	assertChange()

	// Change machine; no change.
	err := s.machine.SetInstanceId("cheese")
	c.Assert(err, IsNil)

	// Assign a unit; change detected.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql0.AssignToMachine(s.machine)
	c.Assert(err, IsNil)
	assertChange("mysql/0")

	// Change the unit; no change.
	err = mysql0.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)
	assertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql1.AssignToMachine(s.machine)
	c.Assert(err, IsNil)
	err = mysql0.Destroy()
	c.Assert(err, IsNil)
	assertChange("mysql/0", "mysql/1")

	// Add a subordinate to the Alive unit; change detected.
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, IsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, IsNil)
	logging0, err := logging.Unit("logging/0")
	c.Assert(err, IsNil)
	assertChange("logging/0")

	// Change the subordinate; no change.
	err = logging0.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)
	assertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, IsNil)
	assertChange("mysql/0")

	// Stop watcher; check Changes chan closed.
	assertClosed := func() {
		select {
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("not closed")
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, false)
		}
	}
	stop(c, w)
	assertClosed()

	// Start a fresh watcher; check all units reported.
	w = s.machine.WatchUnits()
	defer stop(c, w)
	assertChange("mysql/0", "mysql/1", "logging/0")

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, IsNil)
	assertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, IsNil)
	assertChange("logging/0")

	// Unassign the principal; check subordinate departure also reported.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, IsNil)
	assertChange("mysql/1", "logging/0")
}

func (s *MachineSuite) TestAnnotatorForMachine(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestAnnotationRemovalForMachine(c *C) {
	err := s.machine.SetAnnotation("mykey", "myvalue")
	c.Assert(err, IsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
	ann, err := s.machine.Annotations()
	c.Assert(err, IsNil)
	c.Assert(ann, DeepEquals, make(map[string]string))
}
