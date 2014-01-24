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
	jc "launchpad.net/juju-core/testing/checkers"
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
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestContainerDefaults(c *gc.C) {
	c.Assert(string(s.machine.ContainerType()), gc.Equals, "")
	containers, err := s.machine.Containers()
	c.Assert(err, gc.IsNil)
	c.Assert(containers, gc.DeepEquals, []string(nil))
}

func (s *MachineSuite) TestMachineJobFromParams(c *gc.C) {
	for stateMachineJob, paramsMachineJob := range state.JobNames {
		job, err := state.MachineJobFromParams(paramsMachineJob)
		c.Assert(err, gc.IsNil)
		c.Assert(job, gc.Equals, stateMachineJob)
	}
	_, err := state.MachineJobFromParams("invalid")
	c.Assert(err, gc.NotNil)
}

func (s *MachineSuite) TestParentId(c *gc.C) {
	parentId, ok := s.machine.ParentId()
	c.Assert(parentId, gc.Equals, "")
	c.Assert(ok, gc.Equals, false)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	parentId, ok = container.ParentId()
	c.Assert(parentId, gc.Equals, s.machine.Id())
	c.Assert(ok, gc.Equals, true)
}

func (s *MachineSuite) TestMachineIsManager(c *gc.C) {
	tests := []struct {
		isStateServer bool
		jobs          []state.MachineJob
	}{
		{false, []state.MachineJob{state.JobHostUnits}},
		{true, []state.MachineJob{state.JobManageEnviron}},
		{true, []state.MachineJob{state.JobHostUnits, state.JobManageEnviron}},
	}
	for _, test := range tests {
		m, err := s.State.AddMachine("quantal", test.jobs...)
		c.Assert(err, gc.IsNil)
		c.Assert(m.IsManager(), gc.Equals, test.isStateServer)
	}
}

func (s *MachineSuite) TestMachineIsManualBootstrap(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Type(), gc.Not(gc.Equals), "null")
	c.Assert(s.machine.Id(), gc.Equals, "0")
	manual, err := s.machine.IsManual()
	c.Assert(err, gc.IsNil)
	c.Assert(manual, jc.IsFalse)
	newcfg, err := cfg.Apply(map[string]interface{}{"type": "null"})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newcfg, cfg)
	c.Assert(err, gc.IsNil)
	manual, err = s.machine.IsManual()
	c.Assert(err, gc.IsNil)
	c.Assert(manual, jc.IsTrue)
}

func (s *MachineSuite) TestMachineIsManual(c *gc.C) {
	tests := []struct {
		instanceId instance.Id
		nonce      string
		isManual   bool
	}{
		{instanceId: "x", nonce: "y", isManual: false},
		{instanceId: "manual:", nonce: "y", isManual: false},
		{instanceId: "x", nonce: "manual:", isManual: true},
		{instanceId: "x", nonce: "manual:y", isManual: true},
		{instanceId: "x", nonce: "manual", isManual: false},
	}
	for _, test := range tests {
		m, err := s.State.AddOneMachine(state.MachineTemplate{
			Series:     "quantal",
			Jobs:       []state.MachineJob{state.JobHostUnits},
			InstanceId: test.instanceId,
			Nonce:      test.nonce,
		})
		c.Assert(err, gc.IsNil)
		isManual, err := m.IsManual()
		c.Assert(isManual, gc.Equals, test.isManual)
	}
}

func (s *MachineSuite) TestLifeJobManageEnviron(c *gc.C) {
	// A JobManageEnviron machine must never advance lifecycle.
	m, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = m.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 1 is required by the environment")
	err = m.ForceDestroy()
	c.Assert(err, gc.ErrorMatches, "machine 1 is required by the environment")
	err = m.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 1 is required by the environment")
}

func (s *MachineSuite) TestLifeMachineWithContainer(c *gc.C) {
	// A machine hosting a container must not advance lifecycle.
	_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
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
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
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
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
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
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), gc.IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
}

func (s *MachineSuite) TestDestroyContention(c *gc.C) {
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
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
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.HardwareCharacteristics()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	_, err = s.machine.Containers()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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

func (s *MachineSuite) TestSetAgentCompatPassword(c *gc.C) {
	e, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	testSetAgentCompatPassword(c, e)
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
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
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
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
	c.Assert(iid, gc.Equals, instance.Id(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *gc.C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
	c.Assert(string(iid), gc.Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$set", D{{"instanceid", ""}}}},
	)
	c.Assert(err, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
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
	id, err = s.machine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(string(id), gc.Equals, "umbrella/0")
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

func (s *MachineSuite) TestMachineSetInstanceStatus(c *gc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	err = s.machine.SetInstanceStatus("ALIVE")
	c.Assert(err, gc.IsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	status, err := s.machine.InstanceStatus()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.DeepEquals, "ALIVE")
}

func (s *MachineSuite) TestNotProvisionedMachineSetInstanceStatus(c *gc.C) {
	err := s.machine.SetInstanceStatus("ALIVE")
	c.Assert(err, gc.ErrorMatches, ".* not provisioned")
}

func (s *MachineSuite) TestNotProvisionedMachineInstanceStatus(c *gc.C) {
	_, err := s.machine.InstanceStatus()
	c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
}

// SCHEMACHANGE
func (s *MachineSuite) TestInstanceStatusOldSchema(c *gc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	// Remove the InstanceId from instanceData doc to simulate an old schema.
	state.ClearInstanceDocId(c, s.machine)

	err = s.machine.SetInstanceStatus("ALIVE")
	c.Assert(err, gc.IsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	status, err := s.machine.InstanceStatus()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.DeepEquals, "ALIVE")
}

func (s *MachineSuite) TestMachineRefresh(c *gc.C) {
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	oldTools, _ := m0.AgentTools()
	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, gc.IsNil)
	err = m0.SetAgentVersion(version.MustParseBinary("0.0.3-series-arch"))
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
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	m3, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0 := s.AddTestingService(c, "s0", dummy)
	s1 := s.AddTestingService(c, "s1", dummy)
	s2 := s.AddTestingService(c, "s2", dummy)
	s3 := s.AddTestingService(c, "s3", logging)

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
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Clean(), gc.Equals, true)

	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
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
	err = machine.SetAgentVersion(version.MustParseBinary("0.0.3-series-arch"))
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
	//  Machine.WatchHardwareCharacteristics
	//  Service.Watch
	//  Unit.Watch
	//  State.WatchForEnvironConfigChanges
	//  Unit.WatchConfigSettings
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
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
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
	err = mysql0.SetStatus(params.StatusStarted, "", nil)
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
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
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
	err = logging0.SetStatus(params.StatusStarted, "", nil)
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
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, gc.IsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	err = mysql0.SetStatus(params.StatusStarted, "", nil)
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
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
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
	err = logging0.SetStatus(params.StatusStarted, "", nil)
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
	machine1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)

	// Change environment constraints and add a new machine.
	err = s.State.SetEnvironConstraints(econs2)
	c.Assert(err, gc.IsNil)
	machine2, err := s.State.AddMachine("quantal", state.JobHostUnits)
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
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
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
		c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
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
	err := s.machine.SetStatus(params.StatusError, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "error" without info`)
	err = s.machine.SetStatus(params.StatusPending, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "pending"`)
	err = s.machine.SetStatus(params.StatusDown, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "down"`)
	err = s.machine.SetStatus(params.Status("vliegkat"), "orville", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	status, info, data, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.SetStatus(params.StatusError, "provisioning failed", params.StatusData{
		"foo": "bar",
	})
	c.Assert(err, gc.IsNil)
	status, info, data, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, params.StatusData{
		"foo": "bar",
	})
}

func (s *MachineSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	// When Dying set/get should work.
	err := s.machine.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStopped, "", nil)
	c.Assert(err, gc.IsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	// When Dead set should fail, but get will work.
	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	status, info, data, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStopped)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "0": not found or not alive`)
	_, _, _, err = s.machine.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")
}

func (s *MachineSuite) TestGetSetStatusDataStandard(c *gc.C) {
	err := s.machine.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, gc.IsNil)

	// Regular status setting with data.
	err = s.machine.SetStatus(params.StatusError, "provisioning failed", params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
	c.Assert(err, gc.IsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
}

func (s *MachineSuite) TestGetSetStatusDataMongo(c *gc.C) {
	err := s.machine.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, gc.IsNil)

	// Status setting with MongoDB special values.
	err = s.machine.SetStatus(params.StatusError, "mongo", params.StatusData{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
	c.Assert(err, gc.IsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "mongo")
	c.Assert(data, gc.DeepEquals, params.StatusData{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
}

func (s *MachineSuite) TestGetSetStatusDataChange(c *gc.C) {
	err := s.machine.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, gc.IsNil)

	// Status setting and changing data afterwards.
	data := params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	}
	err = s.machine.SetStatus(params.StatusError, "provisioning failed", data)
	c.Assert(err, gc.IsNil)
	data["4th-key"] = 4.0

	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, params.StatusData{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})

	// Set status data to nil, so an empty map will be returned.
	err = s.machine.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)

	status, info, data, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)
}

func (s *MachineSuite) TestSetAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
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

func (s *MachineSuite) TestSetMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := []instance.Address{
		instance.NewAddress("127.0.0.1"),
		instance.NewAddress("8.8.8.8"),
	}
	err = machine.SetMachineAddresses(addresses)
	c.Assert(err, gc.IsNil)
	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(machine.MachineAddresses(), gc.DeepEquals, addresses)
}

func (s *MachineSuite) addMachineWithSupportedContainer(c *gc.C, container instance.ContainerType) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	containers := []instance.ContainerType{container}
	err = machine.SetSupportedContainers(containers)
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, containers)
	return machine
}

// assertSupportedContainers checks the document in memory has the specified
// containers and then reloads the document from the database to assert saved
// values match also.
func assertSupportedContainers(c *gc.C, machine *state.Machine, containers []instance.ContainerType) {
	supportedContainers, known := machine.SupportedContainers()
	c.Assert(known, jc.IsTrue)
	c.Assert(supportedContainers, gc.DeepEquals, containers)
	// Reload so we can check the saved values.
	err := machine.Refresh()
	c.Assert(err, gc.IsNil)
	supportedContainers, known = machine.SupportedContainers()
	c.Assert(known, jc.IsTrue)
	c.Assert(supportedContainers, gc.DeepEquals, containers)
}

func assertSupportedContainersUnknown(c *gc.C, machine *state.Machine) {
	containers, known := machine.SupportedContainers()
	c.Assert(known, jc.IsFalse)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *MachineSuite) TestSupportedContainersInitiallyUnknown(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainers(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = machine.SupportsNoContainers()
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainerTypeNoneIsError(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.NONE})
	c.Assert(err, gc.ErrorMatches, `"none" is not a valid container type`)
	assertSupportedContainersUnknown(c, machine)
	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainersOverwritesExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SupportsNoContainers()
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainersSingle(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC})
}

func (s *MachineSuite) TestSetSupportedContainersSame(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC})
}

func (s *MachineSuite) TestSetSupportedContainersNew(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipeNew(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, gc.IsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersSetsUnknownToError(c *gc.C) {
	// Create a machine and add lxc and kvm containers prior to calling SetSupportedContainers
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	supportedContainer, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, gc.IsNil)
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, gc.IsNil)

	// A supported (kvm) container will have a pending status.
	err = supportedContainer.Refresh()
	c.Assert(err, gc.IsNil)
	status, info, data, err := supportedContainer.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)

	// An unsupported (lxc) container will have an error status.
	err = container.Refresh()
	c.Assert(err, gc.IsNil)
	status, info, data, err = container.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "unsupported container")
	c.Assert(data, gc.DeepEquals, params.StatusData{"type": "lxc"})
}

func (s *MachineSuite) TestSupportsNoContainersSetsAllToError(c *gc.C) {
	// Create a machine and add all container types prior to calling SupportsNoContainers
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	var containers []*state.Machine
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	for _, containerType := range instance.ContainerTypes {
		container, err := s.State.AddMachineInsideMachine(template, machine.Id(), containerType)
		c.Assert(err, gc.IsNil)
		containers = append(containers, container)
	}

	err = machine.SupportsNoContainers()
	c.Assert(err, gc.IsNil)

	// All containers should be in error state.
	for _, container := range containers {
		err = container.Refresh()
		c.Assert(err, gc.IsNil)
		status, info, data, err := container.Status()
		c.Assert(err, gc.IsNil)
		c.Assert(status, gc.Equals, params.StatusError)
		c.Assert(info, gc.Equals, "unsupported container")
		containerType := state.ContainerTypeFromId(container.Id())
		c.Assert(data, gc.DeepEquals, params.StatusData{"type": string(containerType)})
	}
}
