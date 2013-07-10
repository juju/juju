// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
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

func (s *MachineSuite) TestContainerDefaults(c *C) {
	c.Assert(string(s.machine.ContainerType()), Equals, "")
	containers, err := s.machine.Containers()
	c.Assert(err, IsNil)
	c.Assert(containers, DeepEquals, []string(nil))
}

func (s *MachineSuite) TestParentId(c *C) {
	parentId, ok := s.machine.ParentId()
	c.Assert(parentId, Equals, "")
	c.Assert(ok, Equals, false)
	params := state.AddMachineParams{
		ParentId:      s.machine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	parentId, ok = container.ParentId()
	c.Assert(parentId, Equals, s.machine.Id())
	c.Assert(ok, Equals, true)
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

func (s *MachineSuite) TestLifeMachineWithContainer(c *C) {
	// A machine hosting a container must not advance lifecycle.
	params := state.AddMachineParams{
		ParentId:      s.machine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	err = s.machine.Destroy()
	c.Assert(err, FitsTypeOf, &state.HasContainersError{})
	c.Assert(err, ErrorMatches, `machine 0 is hosting containers "0/lxc/0"`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, DeepEquals, err)
	c.Assert(s.machine.Life(), Equals, state.Alive)
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
	c.Assert(err, FitsTypeOf, &state.HasAssignedUnitsError{})
	c.Assert(err, ErrorMatches, `machine 0 has unit "wordpress/0" assigned`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, DeepEquals, err)
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

func (s *MachineSuite) TestDestroyAbort(c *C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Destroy(), IsNil)
	}).Check()
	err := s.machine.Destroy()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestDestroyCancel(c *C) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(err, FitsTypeOf, &state.HasAssignedUnitsError{})
}

func (s *MachineSuite) TestDestroyContention(c *C) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	perturb := state.TransactionHook{
		Before: func() { c.Assert(unit.AssignToMachine(s.machine), IsNil) },
		After:  func() { c.Assert(unit.UnassignFromMachine(), IsNil) },
	}
	defer state.SetTransactionHooks(
		c, s.State, perturb, perturb, perturb,
	).Check()
	err = s.machine.Destroy()
	c.Assert(err, ErrorMatches, "machine 0 cannot advance lifecycle: state changing too quickly; try again soon")
}

func (s *MachineSuite) TestRemove(c *C) {
	err := s.machine.Remove()
	c.Assert(err, ErrorMatches, "cannot remove machine 0: machine is not dead")
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.HardwareCharacteristics()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.Containers()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestRemoveAbort(c *C) {
	err := s.machine.EnsureDead()
	c.Assert(err, IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Remove(), IsNil)
	}).Check()
	err = s.machine.Remove()
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestDestroyMachines(c *C) {
	m0 := s.machine
	m1, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	sch := s.AddTestingCharm(c, "wordpress")
	wordpress, err := s.State.AddService("wordpress", sch)
	c.Assert(err, IsNil)
	u, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m0)
	c.Assert(err, IsNil)

	err = s.State.DestroyMachines("0", "1", "2")
	c.Assert(err, ErrorMatches, `some machines were not destroyed: machine 0 has unit "wordpress/0" assigned; machine 1 is required by the environment`)
	assertLife := func(m *state.Machine, life state.Life) {
		err := m.Refresh()
		c.Assert(err, IsNil)
		c.Assert(m.Life(), Equals, life)
	}
	assertLife(m0, state.Alive)
	assertLife(m1, state.Alive)
	assertLife(m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, IsNil)
	err = s.State.DestroyMachines("0", "1", "2")
	c.Assert(err, ErrorMatches, `some machines were not destroyed: machine 1 is required by the environment`)
	assertLife(m0, state.Dying)
	assertLife(m1, state.Alive)
	assertLife(m2, state.Dying)
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

func (s *MachineSuite) TestTag(c *C) {
	c.Assert(s.machine.Tag(), Equals, "machine-0")
}

func (s *MachineSuite) TestMachineTag(c *C) {
	c.Assert(state.MachineTag("10"), Equals, "machine-10")
	// Check a container id.
	c.Assert(state.MachineTag("10/lxc/1"), Equals, "machine-10-lxc-1")
}

func (s *MachineSuite) TestMachineIdFromTag(c *C) {
	c.Assert(state.MachineIdFromTag("machine-10"), Equals, "10")
	// Check a container id.
	c.Assert(state.MachineIdFromTag("machine-10-lxc-1"), Equals, "10/lxc/1")
	// Check reversability.
	nested := "2/kvm/0/lxc/3"
	c.Assert(state.MachineIdFromTag(state.MachineTag(nested)), Equals, nested)
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
	iid, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(iid, Equals, instance.Id("spaceship/0"))
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
	iid, err := machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
	c.Assert(iid, Equals, instance.Id(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
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
	iid, err := machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
	c.Assert(string(iid), Equals, "")
}

func (s *MachineSuite) TestMachineSetProvisionedUpdatesCharacteristics(c *C) {
	// Before provisioning, there is no hardware characteristics.
	_, err := s.machine.HardwareCharacteristics()
	c.Assert(errors.IsNotFoundError(err), Equals, true)
	arch := "amd64"
	mem := uint64(4096)
	expected := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", expected)
	c.Assert(err, IsNil)
	md, err := s.machine.HardwareCharacteristics()
	c.Assert(err, IsNil)
	c.Assert(*md, DeepEquals, *expected)

	// Reload machine and check again.
	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	md, err = s.machine.HardwareCharacteristics()
	c.Assert(err, IsNil)
	c.Assert(*md, DeepEquals, *expected)
}

func (s *MachineSuite) TestMachineSetCheckProvisioned(c *C) {
	// Check before provisioning.
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), Equals, false)

	// Either one should not be empty.
	err := s.machine.SetProvisioned("umbrella/0", "", nil)
	c.Assert(err, ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "fake_nonce", nil)
	c.Assert(err, ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "", nil)
	c.Assert(err, ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)

	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, IsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, IsNil)
	id, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(string(id), Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), Equals, true)
	// Clear the deprecated machineDoc InstanceId attribute and ensure that CheckProvisioned()
	// still works as expected with the new data model.
	state.SetMachineInstanceId(s.machine, "")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), Equals, true)

	// Try it twice, it should fail.
	err = s.machine.SetProvisioned("doesn't-matter", "phony", nil)
	c.Assert(err, ErrorMatches, `cannot set instance data for machine "0": already set`)

	// Check it with invalid nonce.
	c.Assert(s.machine.CheckProvisioned("not-really"), Equals, false)
}

func (s *MachineSuite) TestMachineSetProvisionedWhenNotAlive(c *C) {
	testWhenDying(c, s.machine, notAliveErr, notAliveErr, func() error {
		return s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	})
}

func (s *MachineSuite) TestMachineRefresh(c *C) {
	m0, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	oldTools, _ := m0.AgentTools()
	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, IsNil)
	err = m0.SetAgentTools(&state.Tools{
		URL:    "foo",
		Binary: version.MustParseBinary("0.0.3-series-arch"),
	})
	c.Assert(err, IsNil)
	newTools, _ := m0.AgentTools()

	m1Tools, _ := m1.AgentTools()
	c.Assert(m1Tools, DeepEquals, oldTools)
	err = m1.Refresh()
	c.Assert(err, IsNil)
	m1Tools, _ = m1.AgentTools()
	c.Assert(*m1Tools, Equals, *newTools)

	err = m0.EnsureDead()
	c.Assert(err, IsNil)
	err = m0.Remove()
	c.Assert(err, IsNil)
	err = m0.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *MachineSuite) TestRefreshWhenNotAlive(c *C) {
	// Refresh should work regardless of liveness status.
	testWhenDying(c, s.machine, noErr, noErr, func() error {
		return s.machine.Refresh()
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

func (s *MachineSuite) assertMachineDirtyAfterAddingUnit(c *C) (*state.Machine, *state.Service, *state.Unit) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	c.Assert(m.Clean(), Equals, true)

	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, IsNil)
	c.Assert(m.Clean(), Equals, false)
	return m, svc, unit
}

func (s *MachineSuite) TestMachineDirtyAfterAddingUnit(c *C) {
	s.assertMachineDirtyAfterAddingUnit(c)
}

func (s *MachineSuite) TestMachineDirtyAfterUnassigningUnit(c *C) {
	m, _, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	c.Assert(m.Clean(), Equals, false)
}

func (s *MachineSuite) TestMachineDirtyAfterRemovingUnit(c *C) {
	m, svc, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	err = svc.Destroy()
	c.Assert(err, IsNil)
	c.Assert(m.Clean(), Equals, false)
}

func (s *MachineSuite) TestWatchMachine(c *C) {
	w := s.machine.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, IsNil)
	err = machine.SetProvisioned("m-foo", "fake_nonce", nil)
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = machine.SetAgentTools(&state.Tools{
		URL:    "foo",
		Binary: version.MustParseBinary("0.0.3-series-arch"),
	})
	c.Assert(err, IsNil)
	err = machine.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove machine, start new watch, check single event.
	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	err = machine.Remove()
	c.Assert(err, IsNil)
	w = s.machine.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Change machine, and create a unit independently; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, IsNil)
	wc.AssertNoChange()
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Assign that unit (to a separate machine instance); change detected.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, IsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0")

	// Change the unit; no change.
	err = mysql0.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, IsNil)
	err = mysql0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0", "mysql/1")

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
	wc.AssertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0")

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check both principals reported.
	w = s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange("mysql/0", "mysql/1")

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Unassign the unit; check change.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/1")
}

func (s *MachineSuite) TestWatchUnits(c *C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Change machine; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Assign a unit (to a separate instance); change detected.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, IsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0")

	// Change the unit; no change.
	err = mysql0.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, IsNil)
	err = mysql0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0", "mysql/1")

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
	wc.AssertOneChange("logging/0")

	// Change the subordinate; no change.
	err = logging0.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/0")

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check all units reported.
	w = s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewLaxStringsWatcherC(c, s.State, w)
	wc.AssertOneChange("mysql/0", "mysql/1", "logging/0")

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; change detected.
	err = logging0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertOneChange("logging/0")

	// Unassign the principal; check subordinate departure also reported.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, IsNil)
	wc.AssertOneChange("mysql/1", "logging/0")
}

func (s *MachineSuite) TestAnnotatorForMachine(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestAnnotationRemovalForMachine(c *C) {
	annotations := map[string]string{"mykey": "myvalue"}
	err := s.machine.SetAnnotations(annotations)
	c.Assert(err, IsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)
	ann, err := s.machine.Annotations()
	c.Assert(err, IsNil)
	c.Assert(ann, DeepEquals, make(map[string]string))
}

func (s *MachineSuite) TestConstraintsFromEnvironment(c *C) {
	econs1 := constraints.MustParse("mem=1G")
	econs2 := constraints.MustParse("mem=2G")

	// A newly-created machine gets a copy of the environment constraints.
	err := s.State.SetEnvironConstraints(econs1)
	c.Assert(err, IsNil)
	machine1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons1, DeepEquals, econs1)

	// Change environment constraints and add a new machine.
	err = s.State.SetEnvironConstraints(econs2)
	c.Assert(err, IsNil)
	machine2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	mcons2, err := machine2.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons2, DeepEquals, econs2)

	// Check the original machine has its original constraints.
	mcons1, err = machine1.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons1, DeepEquals, econs1)
}

func (s *MachineSuite) TestSetConstraints(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	// Constraints can be set...
	cons1 := constraints.MustParse("mem=1G")
	err = machine.SetConstraints(cons1)
	c.Assert(err, IsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, cons1)

	// ...until the machine is provisioned, at which point they stick.
	err = machine.SetProvisioned("i-mstuck", "fake_nonce", nil)
	c.Assert(err, IsNil)
	cons2 := constraints.MustParse("mem=2G")
	err = machine.SetConstraints(cons2)
	c.Assert(err, ErrorMatches, "cannot set constraints: machine is already provisioned")

	// Check the failed set had no effect.
	mcons, err = machine.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, cons1)
}

func (s *MachineSuite) TestConstraintsLifecycle(c *C) {
	cons := constraints.MustParse("mem=1G")
	cannotSet := `cannot set constraints: not found or not alive`
	testWhenDying(c, s.machine, cannotSet, cannotSet, func() error {
		err := s.machine.SetConstraints(cons)
		mcons, err1 := s.machine.Constraints()
		c.Assert(err1, IsNil)
		c.Assert(mcons, DeepEquals, constraints.Value{})
		return err
	})

	err := s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.SetConstraints(cons)
	c.Assert(err, ErrorMatches, cannotSet)
	_, err = s.machine.Constraints()
	c.Assert(err, ErrorMatches, `constraints not found`)
}

func (s *MachineSuite) TestGetSetStatusWhileAlive(c *C) {
	failError := func() { s.machine.SetStatus(params.StatusError, "") }
	c.Assert(failError, PanicMatches, "machine error status with no info")
	failPending := func() { s.machine.SetStatus(params.StatusPending, "") }
	c.Assert(failPending, PanicMatches, "machine status cannot be set to pending")

	status, info, err := s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	err = s.machine.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	status, info, err = s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStarted)
	c.Assert(info, Equals, "")

	err = s.machine.SetStatus(params.StatusError, "provisioning failed")
	c.Assert(err, IsNil)
	status, info, err = s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusError)
	c.Assert(info, Equals, "provisioning failed")
}

func (s *MachineSuite) TestGetSetStatusWhileNotAlive(c *C) {
	// When Dying set/get should work.
	err := s.machine.Destroy()
	c.Assert(err, IsNil)
	err = s.machine.SetStatus(params.StatusStopped, "")
	c.Assert(err, IsNil)
	status, info, err := s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStopped)
	c.Assert(info, Equals, "")

	// When Dead set should fail, but get will work.
	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	status, info, err = s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStopped)
	c.Assert(info, Equals, "")

	err = s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	_, _, err = s.machine.Status()
	c.Assert(err, ErrorMatches, "status not found")
}
