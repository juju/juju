// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type MachineSuite struct {
	ConnSuite
	machine *state.Machine
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestContainerDefaults(c *gc.C) {
	c.Assert(string(s.machine.ContainerType()), gc.Equals, "")
	containers, err := s.machine.Containers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.DeepEquals, []string(nil))
}

func (s *MachineSuite) TestParentId(c *gc.C) {
	parentId, ok := s.machine.ParentId()
	c.Assert(parentId, gc.Equals, "")
	c.Assert(ok, gc.Equals, false)
	params := state.AddMachineParams{
		ParentId:      s.machine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	parentId, ok = container.ParentId()
	c.Assert(parentId, gc.Equals, s.machine.Id())
	c.Assert(ok, gc.Equals, true)
}

func (s *MachineSuite) TestMachineIsStateServer(c *gc.C) {
	tests := []struct {
		isStateServer bool
		jobs          []state.MachineJob
	}{
		{false, []state.MachineJob{state.JobHostUnits}},
		{false, []state.MachineJob{state.JobHostUnits, state.JobManageEnviron}},
		{true, []state.MachineJob{state.JobHostUnits, state.JobManageState, state.JobManageEnviron}},
		{true, []state.MachineJob{state.JobManageState}},
	}
	for _, test := range tests {
		params := state.AddMachineParams{
			Series: "series",
			Jobs:   test.jobs,
		}
		m, err := s.State.AddMachineWithConstraints(&params)
		c.Assert(err, gc.IsNil)
		c.Assert(m.IsStateServer(), gc.Equals, test.isStateServer)
	}
}

func (s *MachineSuite) TestLifeJobManageEnviron(c *gc.C) {
	// A JobManageEnviron machine must never advance lifecycle.
	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = m.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 1 is required by the environment")
	err = m.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 1 is required by the environment")
}

func (s *MachineSuite) TestLifeMachineWithContainer(c *gc.C) {
	// A machine hosting a container must not advance lifecycle.
	params := state.AddMachineParams{
		ParentId:      s.machine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasContainersError{})
	c.Assert(err, gc.ErrorMatches, `machine 0 is hosting containers "0/lxc/0"`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, gc.DeepEquals, err)
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)
}

func (s *MachineSuite) TestLifeJobHostUnits(c *gc.C) {
	// A machine with an assigned unit must not advance lifecycle.
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
	c.Assert(err, gc.ErrorMatches, `machine 0 has unit "wordpress/0" assigned`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, gc.DeepEquals, err)
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)

	// Once no unit is assigned, lifecycle can advance.
	err = unit.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	err = s.machine.Destroy()
	c.Assert(s.machine.Life(), gc.Equals, state.Dying)
	c.Assert(err, gc.IsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)

	// A machine that has never had units assigned can advance lifecycle.
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = m.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestDestroyAbort(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Destroy(), gc.IsNil)
	}).Check()
	err := s.machine.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestDestroyCancel(c *gc.C) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), gc.IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
}

func (s *MachineSuite) TestDestroyContention(c *gc.C) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	perturb := state.TransactionHook{
		Before: func() { c.Assert(unit.AssignToMachine(s.machine), gc.IsNil) },
		After:  func() { c.Assert(unit.UnassignFromMachine(), gc.IsNil) },
	}
	defer state.SetTransactionHooks(
		c, s.State, perturb, perturb, perturb,
	).Check()
	err = s.machine.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 cannot advance lifecycle: state changing too quickly; try again soon")
}

func (s *MachineSuite) TestRemove(c *gc.C) {
	err := s.machine.Remove()
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 0: machine is not dead")
	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
	err = s.machine.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.HardwareCharacteristics()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.Containers()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestRemoveAbort(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Remove(), gc.IsNil)
	}).Check()
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestDestroyMachines(c *gc.C) {
	m0 := s.machine
	m1, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	m2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	sch := s.AddTestingCharm(c, "wordpress")
	wordpress, err := s.State.AddService("wordpress", sch)
	c.Assert(err, gc.IsNil)
	u, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m0)
	c.Assert(err, gc.IsNil)

	err = s.State.DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 has unit "wordpress/0" assigned; machine 1 is required by the environment`)
	assertLife := func(m *state.Machine, life state.Life) {
		err := m.Refresh()
		c.Assert(err, gc.IsNil)
		c.Assert(m.Life(), gc.Equals, life)
	}
	assertLife(m0, state.Alive)
	assertLife(m1, state.Alive)
	assertLife(m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	err = s.State.DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 1 is required by the environment`)
	assertLife(m0, state.Dying)
	assertLife(m1, state.Alive)
	assertLife(m2, state.Dying)
}

func (s *MachineSuite) TestMachineSetAgentAlive(c *gc.C) {
	alive, err := s.machine.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(pinger, gc.NotNil)
	defer pinger.Stop()

	s.State.StartSync()
	alive, err = s.machine.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, true)
}

func (s *MachineSuite) TestTag(c *gc.C) {
	c.Assert(s.machine.Tag(), gc.Equals, "machine-0")
}

func (s *MachineSuite) TestSetMongoPassword(c *gc.C) {
	testSetMongoPassword(c, func(st *state.State) (entity, error) {
		return st.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestSetPassword(c *gc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestMachineWaitAgentAlive(c *gc.C) {
	alive, err := s.machine.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)

	s.State.StartSync()
	err = s.machine.WaitAgentAlive(coretesting.ShortWait)
	c.Assert(err, gc.ErrorMatches, `waiting for agent of machine 0: still not alive after timeout`)

	pinger, err := s.machine.SetAgentAlive()
	c.Assert(err, gc.IsNil)

	s.State.StartSync()
	err = s.machine.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, gc.IsNil)

	alive, err = s.machine.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, true)

	err = pinger.Kill()
	c.Assert(err, gc.IsNil)

	s.State.StartSync()
	alive, err = s.machine.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)
}

func (s *MachineSuite) TestMachineInstanceId(c *gc.C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", "spaceship/0"}}}},
	)
	c.Assert(err, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(iid, gc.Equals, instance.Id("spaceship/0"))
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *gc.C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
	c.Assert(iid, gc.Equals, instance.Id(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *gc.C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
	c.Assert(string(iid), gc.Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *gc.C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", ""}}}},
	)
	c.Assert(err, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, checkers.Satisfies, state.IsNotProvisionedError)
	c.Assert(string(iid), gc.Equals, "")
}

func (s *MachineSuite) TestMachineSetProvisionedUpdatesCharacteristics(c *gc.C) {
	// Before provisioning, there is no hardware characteristics.
	_, err := s.machine.HardwareCharacteristics()
	c.Assert(errors.IsNotFoundError(err), gc.Equals, true)
	arch := "amd64"
	mem := uint64(4096)
	expected := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", expected)
	c.Assert(err, gc.IsNil)
	md, err := s.machine.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(*md, gc.DeepEquals, *expected)

	// Reload machine and check again.
	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	md, err = s.machine.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(*md, gc.DeepEquals, *expected)
}

func (s *MachineSuite) TestMachineSetCheckProvisioned(c *gc.C) {
	// Check before provisioning.
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), gc.Equals, false)

	// Either one should not be empty.
	err := s.machine.SetProvisioned("umbrella/0", "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "fake_nonce", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "0": instance id and nonce cannot be empty`)

	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	id, err := m.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(string(id), gc.Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), gc.Equals, true)
	// Clear the deprecated machineDoc InstanceId attribute and ensure that CheckProvisioned()
	// still works as expected with the new data model.
	state.SetMachineInstanceId(s.machine, "")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), gc.Equals, true)

	// Try it twice, it should fail.
	err = s.machine.SetProvisioned("doesn't-matter", "phony", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "0": already set`)

	// Check it with invalid nonce.
	c.Assert(s.machine.CheckProvisioned("not-really"), gc.Equals, false)
}

func (s *MachineSuite) TestMachineSetProvisionedWhenNotAlive(c *gc.C) {
	testWhenDying(c, s.machine, notAliveErr, notAliveErr, func() error {
		return s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	})
}

func (s *MachineSuite) TestMachineRefresh(c *gc.C) {
	m0, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	oldTools, _ := m0.AgentTools()
	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, gc.IsNil)
	err = m0.SetAgentTools(&tools.Tools{
		URL:     "foo",
		Version: version.MustParseBinary("0.0.3-series-arch"),
	})
	c.Assert(err, gc.IsNil)
	newTools, _ := m0.AgentTools()

	m1Tools, _ := m1.AgentTools()
	c.Assert(m1Tools, gc.DeepEquals, oldTools)
	err = m1.Refresh()
	c.Assert(err, gc.IsNil)
	m1Tools, _ = m1.AgentTools()
	c.Assert(*m1Tools, gc.Equals, *newTools)

	err = m0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = m0.Remove()
	c.Assert(err, gc.IsNil)
	err = m0.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *MachineSuite) TestRefreshWhenNotAlive(c *gc.C) {
	// Refresh should work regardless of liveness status.
	testWhenDying(c, s.machine, noErr, noErr, func() error {
		return s.machine.Refresh()
	})
}

func (s *MachineSuite) TestMachinePrincipalUnits(c *gc.C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	m3, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0, err := s.State.AddService("s0", dummy)
	c.Assert(err, gc.IsNil)
	s1, err := s.State.AddService("s1", dummy)
	c.Assert(err, gc.IsNil)
	s2, err := s.State.AddService("s2", dummy)
	c.Assert(err, gc.IsNil)
	s3, err := s.State.AddService("s3", logging)
	c.Assert(err, gc.IsNil)

	units := make([][]*state.Unit, 4)
	for i, svc := range []*state.Service{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = svc.AddUnit()
			c.Assert(err, gc.IsNil)
		}
	}
	// Add the logging units subordinate to the s2 units.
	eps, err := s.State.InferEndpoints([]string{"s2", "s3"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	for _, u := range units[2] {
		ru, err := rel.Unit(u)
		c.Assert(err, gc.IsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, gc.IsNil)
	}
	units[3], err = s3.AllUnits()
	c.Assert(err, gc.IsNil)

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
			c.Assert(err, gc.IsNil)
		}
	}

	for i, a := range assignments {
		c.Logf("test %d", i)
		got, err := a.machine.Units()
		c.Assert(err, gc.IsNil)
		expect := sortedUnitNames(append(a.units, a.subordinates...))
		c.Assert(sortedUnitNames(got), gc.DeepEquals, expect)
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

func (s *MachineSuite) assertMachineDirtyAfterAddingUnit(c *gc.C) (*state.Machine, *state.Service, *state.Unit) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Clean(), gc.Equals, true)

	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Clean(), gc.Equals, false)
	return m, svc, unit
}

func (s *MachineSuite) TestMachineDirtyAfterAddingUnit(c *gc.C) {
	s.assertMachineDirtyAfterAddingUnit(c)
}

func (s *MachineSuite) TestMachineDirtyAfterUnassigningUnit(c *gc.C) {
	m, _, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Clean(), gc.Equals, false)
}

func (s *MachineSuite) TestMachineDirtyAfterRemovingUnit(c *gc.C) {
	m, svc, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = unit.Remove()
	c.Assert(err, gc.IsNil)
	err = svc.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Clean(), gc.Equals, false)
}

func (s *MachineSuite) TestWatchMachine(c *gc.C) {
	w := s.machine.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("m-foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = machine.SetAgentTools(&tools.Tools{
		URL:     "foo",
		Version: version.MustParseBinary("0.0.3-series-arch"),
	})
	c.Assert(err, gc.IsNil)
	err = machine.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove machine, start new watch, check single event.
	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = machine.Remove()
	c.Assert(err, gc.IsNil)
	w = s.machine.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *MachineSuite) TestWatchDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.entityWatcher, which
	// is also used by:
	//	Machine.WatchHardwareCharacteristics
	//	Service.Watch
	//	Unit.Watch
	//	State.WatchForEnvironConfigChanges
	//	Unit.WatchConfigSettings
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, gc.IsNil)
		w := m.Watch()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *gc.C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine, and create a unit independently; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Assign that unit (to a separate machine instance); change detected.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	err = mysql0.SetStatus(params.StatusStarted, "")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	err = mysql0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; no change.
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, gc.IsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	logging0, err := logging.Unit("logging/0")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetStatus(params.StatusStarted, "")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check both principals reported.
	w = s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Unassign the unit; check change.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/1")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchPrincipalUnitsDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.unitsWatcher, which
	// is also used by Unit.WatchSubordinateUnits.
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, gc.IsNil)
		w := m.WatchPrincipalUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchUnits(c *gc.C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Assign a unit (to a separate instance); change detected.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	err = mysql0.SetStatus(params.StatusStarted, "")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	err = mysql0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; change detected.
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, gc.IsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	logging0, err := logging.Unit("logging/0")
	c.Assert(err, gc.IsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetStatus(params.StatusStarted, "")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check all units reported.
	w = s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql/0", "mysql/1", "logging/0")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; change detected.
	err = logging0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Unassign the principal; check subordinate departure also reported.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/1", "logging/0")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchUnitsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, gc.IsNil)
		w := m.WatchUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestAnnotatorForMachine(c *gc.C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestAnnotationRemovalForMachine(c *gc.C) {
	annotations := map[string]string{"mykey": "myvalue"}
	err := s.machine.SetAnnotations(annotations)
	c.Assert(err, gc.IsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
	ann, err := s.machine.Annotations()
	c.Assert(err, gc.IsNil)
	c.Assert(ann, gc.DeepEquals, make(map[string]string))
}

func (s *MachineSuite) TestConstraintsFromEnvironment(c *gc.C) {
	econs1 := constraints.MustParse("mem=1G")
	econs2 := constraints.MustParse("mem=2G")

	// A newly-created machine gets a copy of the environment constraints.
	err := s.State.SetEnvironConstraints(econs1)
	c.Assert(err, gc.IsNil)
	machine1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)

	// Change environment constraints and add a new machine.
	err = s.State.SetEnvironConstraints(econs2)
	c.Assert(err, gc.IsNil)
	machine2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	mcons2, err := machine2.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons2, gc.DeepEquals, econs2)

	// Check the original machine has its original constraints.
	mcons1, err = machine1.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)
}

func (s *MachineSuite) TestSetConstraints(c *gc.C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// Constraints can be set...
	cons1 := constraints.MustParse("mem=1G")
	err = machine.SetConstraints(cons1)
	c.Assert(err, gc.IsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons, gc.DeepEquals, cons1)

	// ...until the machine is provisioned, at which point they stick.
	err = machine.SetProvisioned("i-mstuck", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	cons2 := constraints.MustParse("mem=2G")
	err = machine.SetConstraints(cons2)
	c.Assert(err, gc.ErrorMatches, "cannot set constraints: machine is already provisioned")

	// Check the failed set had no effect.
	mcons, err = machine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons, gc.DeepEquals, cons1)
}

func (s *MachineSuite) TestConstraintsLifecycle(c *gc.C) {
	cons := constraints.MustParse("mem=1G")
	cannotSet := `cannot set constraints: not found or not alive`
	testWhenDying(c, s.machine, cannotSet, cannotSet, func() error {
		err := s.machine.SetConstraints(cons)
		mcons, err1 := s.machine.Constraints()
		c.Assert(err1, gc.IsNil)
		c.Assert(mcons, gc.DeepEquals, constraints.Value{})
		return err
	})

	err := s.machine.Remove()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, cannotSet)
	_, err = s.machine.Constraints()
	c.Assert(err, gc.ErrorMatches, `constraints not found`)
}

func (s *MachineSuite) TestGetSetStatusWhileAlive(c *gc.C) {
	err := s.machine.SetStatus(params.StatusError, "")
	c.Assert(err, gc.ErrorMatches, `cannot set status "error" without info`)
	err = s.machine.SetStatus(params.StatusPending, "")
	c.Assert(err, gc.ErrorMatches, `cannot set status "pending"`)
	err = s.machine.SetStatus(params.StatusDown, "")
	c.Assert(err, gc.ErrorMatches, `cannot set status "down"`)
	err = s.machine.SetStatus(params.Status("vliegkat"), "orville")
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	status, info, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = s.machine.SetStatus(params.StatusStarted, "")
	c.Assert(err, gc.IsNil)
	status, info, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "")

	err = s.machine.SetStatus(params.StatusError, "provisioning failed")
	c.Assert(err, gc.IsNil)
	status, info, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
}

func (s *MachineSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	// When Dying set/get should work.
	err := s.machine.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStopped, "")
	c.Assert(err, gc.IsNil)
	status, info, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
	c.Assert(info, gc.Equals, "")

	// When Dead set should fail, but get will work.
	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	status, info, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
	c.Assert(info, gc.Equals, "")

	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	_, _, err = s.machine.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")
}

func (s *MachineSuite) TestSetAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := []instance.Address{
		instance.NewAddress("127.0.0.1"),
		instance.NewAddress("8.8.8.8"),
	}
	err = machine.SetAddresses(addresses)
	c.Assert(err, gc.IsNil)
	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Addresses(), gc.DeepEquals, addresses)
}
