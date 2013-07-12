// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	. "launchpad.net/juju-core/testing/checkers"
	"sort"
	"strconv"
	"time"
)

type AssignSuite struct {
	ConnSuite
	wordpress *state.Service
}

var _ = Suite(&AssignSuite{})
var _ = Suite(&assignCleanSuite{ConnSuite{}, state.AssignCleanEmpty, nil})
var _ = Suite(&assignCleanSuite{ConnSuite{}, state.AssignClean, nil})

func (s *AssignSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	s.wordpress = wordpress
}

func (s *AssignSuite) addSubordinate(c *C, principal *state.Unit) *state.Unit {
	_, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(principal)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)
	return subUnit
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	// Check that the unit has no machine assigned.
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToMachineAgainFails(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	machineTwo, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to a different machine should fail.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to machine 1: unit is already assigned to a machine`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, "0")
}

func (s *AssignSuite) TestAssignedMachineIdWhenNotAlive(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	testWhenDying(c, unit, noErr, noErr,
		func() error {
			_, err = unit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestAssignedMachineIdWhenPrincipalNotAlive(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	subUnit := s.addSubordinate(c, unit)
	err = unit.Destroy()
	c.Assert(err, IsNil)
	mid, err := subUnit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, machine.Id())
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)

	err = s.wordpress.Destroy()
	c.Assert(err, IsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *C) {
	// Check that assigning a principal unit assigns its subordinates too.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	subUnit := s.addSubordinate(c, unit)

	// None of the direct unit assign methods work on subordinates.
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = subUnit.AssignToMachine(machine)
	c.Assert(err, ErrorMatches, `cannot assign unit "logging/0" to machine 0: unit is a subordinate`)
	_, err = subUnit.AssignToCleanMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "logging/0" to clean machine: unit is a subordinate`)
	_, err = subUnit.AssignToCleanEmptyMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "logging/0" to clean, empty machine: unit is a subordinate`)
	err = subUnit.AssignToNewMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "logging/0" to new machine: unit is a subordinate`)

	// Subordinates know the machine they're indirectly assigned to.
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	id, err := subUnit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Check(id, Equals, machine.Id())

	// Unassigning the principal unassigns the subordinates too.
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	_, err = subUnit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `unit "logging/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestDeployerTag(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	principal, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	subordinate := s.addSubordinate(c, principal)

	assertDeployer := func(u *state.Unit, d state.Tagger) {
		err := u.Refresh()
		c.Assert(err, IsNil)
		name, ok := u.DeployerTag()
		if d == nil {
			c.Assert(ok, IsFalse)
		} else {
			c.Assert(ok, IsTrue)
			c.Assert(name, Equals, d.Tag())
		}
	}
	assertDeployer(subordinate, principal)
	assertDeployer(principal, nil)

	err = principal.AssignToMachine(machine)
	c.Assert(err, IsNil)
	assertDeployer(subordinate, principal)
	assertDeployer(principal, machine)

	err = principal.UnassignFromMachine()
	c.Assert(err, IsNil)
	assertDeployer(subordinate, principal)
	assertDeployer(principal, nil)
}

func (s *AssignSuite) TestDirectAssignIgnoresConstraints(c *C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	econs := constraints.MustParse("mem=4G cpu-cores=2")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// Machine will take environment constraints on creation.
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	// Unit will take combined service/environ constraints on creation.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Machine keeps its original constraints on direct assignment.
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, IsNil)
	c.Assert(mcons, DeepEquals, econs)
}

func (s *AssignSuite) TestAssignBadSeries(c *C) {
	machine, err := s.State.AddMachine("burble", state.JobHostUnits)
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to machine 0: series does not match`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	subUnit := s.addSubordinate(c, unit)
	assignTest := func() error {
		err := unit.AssignToMachine(machine)
		c.Assert(unit.UnassignFromMachine(), IsNil)
		if subUnit != nil {
			err := subUnit.EnsureDead()
			c.Assert(err, IsNil)
			err = subUnit.Remove()
			c.Assert(err, IsNil)
			subUnit = nil
		}
		return err
	}
	expect := ".*: unit is not alive"
	testWhenDying(c, unit, expect, expect, assignTest)

	expect = ".*: machine is not alive"
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	testWhenDying(c, machine, expect, expect, assignTest)
}

func (s *AssignSuite) TestAssignMachinePrincipalsChange(c *C) {
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	subUnit := s.addSubordinate(c, unit)

	doc := make(map[string][]string)
	s.ConnSuite.machines.FindId(machine.Id()).One(&doc)
	principals, ok := doc["principals"]
	if !ok {
		c.Errorf(`machine document does not have a "principals" field`)
	}
	c.Assert(principals, DeepEquals, []string{"wordpress/0", "wordpress/1"})

	err = subUnit.EnsureDead()
	c.Assert(err, IsNil)
	err = subUnit.Remove()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	doc = make(map[string][]string)
	s.ConnSuite.machines.FindId(machine.Id()).One(&doc)
	principals, ok = doc["principals"]
	if !ok {
		c.Errorf(`machine document does not have a "principals" field`)
	}
	c.Assert(principals, DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) assertAssignedUnit(c *C, unit *state.Unit) string {
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	// Check that the principal is set on the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, IsNil)
	machineUnits, err := machine.Units()
	c.Assert(err, IsNil)
	c.Assert(machineUnits, HasLen, 1)
	// Make sure it is the right unit.
	c.Assert(machineUnits[0].Name(), Equals, unit.Name())
	return machineId
}

func (s *AssignSuite) TestAssignUnitToNewMachine(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	s.assertAssignedUnit(c, unit)
}

func (s *AssignSuite) assertAssignUnitToNewMachineContainerConstraint(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	machineId := s.assertAssignedUnit(c, unit)
	c.Assert(state.ParentId(machineId), Not(Equals), "")
	c.Assert(state.ContainerTypeFromId(machineId), Equals, instance.LXC)
}

func (s *AssignSuite) TestAssignUnitToNewMachineContainerConstraint(c *C) {
	// Set up service constraints.
	scons := constraints.MustParse("container=lxc")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitToNewMachineDefaultContainerConstraint(c *C) {
	// Set up env constraints.
	econs := constraints.MustParse("container=lxc")
	err := s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignToNewMachineMakesDirty(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, IsNil)
	c.Assert(machine.Clean(), IsFalse)
}

func (s *AssignSuite) TestAssignUnitToNewMachineSetsConstraints(c *C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	econs := constraints.MustParse("mem=4G cpu-cores=2")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// Unit will take combined service/environ constraints on creation.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Change service/env constraints before assigning, to verify this.
	scons = constraints.MustParse("mem=6G cpu-power=800")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	econs = constraints.MustParse("cpu-cores=4")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// The new machine takes the original combined unit constraints.
	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	err = unit.Refresh()
	c.Assert(err, IsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, IsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, IsNil)
	expect := constraints.MustParse("mem=2G cpu-cores=2 cpu-power=400")
	c.Assert(mcons, DeepEquals, expect)
}

func (s *AssignSuite) TestAssignUnitToNewMachineCleanAvailable(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Add a clean machine.
	clean, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	// Check that the machine isn't our clean one.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Not(Equals), clean.Id())
}

func (s *AssignSuite) TestAssignUnitToNewMachineAlreadyAssigned(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Make the unit assigned
	err = unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	// Try to assign it again
	err = unit.AssignToNewMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is already assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitNotAlive(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	subUnit := s.addSubordinate(c, unit)

	// Try to assign a dying unit...
	err = unit.Destroy()
	c.Assert(err, IsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not alive`)

	// ...and a dead one.
	err = subUnit.EnsureDead()
	c.Assert(err, IsNil)
	err = subUnit.Remove()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not alive`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitRemoved(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = unit.Destroy()
	c.Assert(err, IsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit not found`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesDirty(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// Create some units and a clean machine.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	anotherUnit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	makeDirty := state.TransactionHook{
		Before: func() { c.Assert(unit.AssignToMachine(machine), IsNil) },
	}
	defer state.SetTransactionHooks(
		c, s.State, makeDirty,
	).Check()

	err = anotherUnit.AssignToNewMachineOrContainer()
	c.Assert(err, IsNil)

	mid, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, "1")

	mid, err = anotherUnit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, "2/lxc/0")
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesHost(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// Create a unit and a clean machine.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	addContainer := state.TransactionHook{
		Before: func() {
			params := &state.AddMachineParams{
				Series:        "series",
				ParentId:      machine.Id(),
				ContainerType: instance.LXC,
				Jobs:          []state.MachineJob{state.JobHostUnits},
			}
			_, err := s.State.AddMachineWithConstraints(params)
			c.Assert(err, IsNil)
		},
	}
	defer state.SetTransactionHooks(
		c, s.State, addContainer,
	).Check()

	err = unit.AssignToNewMachineOrContainer()
	c.Assert(err, IsNil)

	mid, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, "2/lxc/0")
}

func (s *AssignSuite) TestAssignUnitBadPolicy(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Check nonsensical policy
	err = s.State.AssignUnit(unit, state.AssignmentPolicy("random"))
	c.Assert(err, ErrorMatches, `.*unknown unit assignment policy: "random"`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, NotNil)
	assertMachineCount(c, s.State, 0)
}

func (s *AssignSuite) TestAssignUnitLocalPolicy(c *C) {
	m, err := s.State.AddMachine("series", state.JobManageEnviron, state.JobHostUnits) // bootstrap machine
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	for i := 0; i < 2; i++ {
		err = s.State.AssignUnit(unit, state.AssignLocal)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		c.Assert(mid, Equals, m.Id())
		assertMachineCount(c, s.State, 1)
	}
}

func (s *AssignSuite) assertAssignUnitNewPolicyNoContainer(c *C) {
	_, err := s.State.AddMachine("series", state.JobHostUnits) // available machine
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, IsNil)
	assertMachineCount(c, s.State, 2)
	id, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(state.ParentId(id), Equals, "")
}

func (s *AssignSuite) TestAssignUnitNewPolicy(c *C) {
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraintIgnoresNone(c *C) {
	scons := constraints.MustParse("container=none")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) assertAssignUnitNewPolicyWithContainerConstraint(c *C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, IsNil)
	assertMachineCount(c, s.State, 3)
	id, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "1/lxc/0")
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraint(c *C) {
	_, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	// Set up service constraints.
	scons := constraints.MustParse("container=lxc")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, IsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithDefaultContainerConstraint(c *C) {
	_, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	// Set up env constraints.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitWithSubordinate(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check cannot assign subordinates to machines
	subUnit := s.addSubordinate(c, unit)
	for _, policy := range []state.AssignmentPolicy{
		state.AssignLocal, state.AssignNew, state.AssignClean, state.AssignCleanEmpty,
	} {
		err = s.State.AssignUnit(subUnit, policy)
		c.Assert(err, ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
	}
}

func assertMachineCount(c *C, st *state.State, expect int) {
	ms, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ms, HasLen, expect, Commentf("%v", ms))
}

// assignCleanSuite has tests for assigning units to 1. clean, and 2. clean&empty machines.
type assignCleanSuite struct {
	ConnSuite
	policy    state.AssignmentPolicy
	wordpress *state.Service
}

func (s *assignCleanSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	s.wordpress = wordpress
}

func (s *assignCleanSuite) errorMessage(msg string) string {
	context := "clean"
	if s.policy == state.AssignCleanEmpty {
		context += ", empty"
	}
	return fmt.Sprintf(msg, context)
}

func (s *assignCleanSuite) assignUnit(unit *state.Unit) (*state.Machine, error) {
	if s.policy == state.AssignCleanEmpty {
		return unit.AssignToCleanEmptyMachine()
	}
	return unit.AssignToCleanMachine()
}

func (s *assignCleanSuite) assertMachineEmpty(c *C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, IsNil)
	c.Assert(len(containers), Equals, 0)
}

func (s *assignCleanSuite) assertMachineNotEmpty(c *C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, IsNil)
	c.Assert(len(containers), Not(Equals), 0)
}

// setupMachines creates a combination of machines with which to test.
func (s *assignCleanSuite) setupMachines(c *C) (hostMachine *state.Machine, container *state.Machine, cleanEmptyMachine *state.Machine) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)

	// Add some units to another service and allocate them to machines
	service1, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	units := make([]*state.Unit, 3)
	for i := range units {
		u, err := service1.AddUnit()
		c.Assert(err, IsNil)
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		err = u.AssignToMachine(m)
		c.Assert(err, IsNil)
		units[i] = u
	}

	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	params := state.AddMachineParams{
		ParentId:      hostMachine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(hostMachine.Clean(), IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	// Create a new, clean, empty machine.
	cleanEmptyMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	c.Assert(cleanEmptyMachine.Clean(), IsTrue)
	s.assertMachineEmpty(c, cleanEmptyMachine)
	return hostMachine, container, cleanEmptyMachine
}

func (s *assignCleanSuite) assertAssignUnit(c *C, expectedMachine *state.Machine) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	reusedMachine, err := s.assignUnit(unit)
	c.Assert(err, IsNil)
	c.Assert(reusedMachine.Id(), Equals, expectedMachine.Id())
	c.Assert(reusedMachine.Clean(), IsFalse)
}

func (s *assignCleanSuite) TestAssignUnit(c *C) {
	hostMachine, container, cleanEmptyMachine := s.setupMachines(c)
	// Check that AssignToClean(Empty)Machine finds a newly created, clean (maybe empty) machine.
	if s.policy == state.AssignCleanEmpty {
		// The first clean, empty machine is the container.
		s.assertAssignUnit(c, container)
		// The next deployment will use the remaining clean, empty machine.
		s.assertAssignUnit(c, cleanEmptyMachine)
	} else {
		s.assertAssignUnit(c, hostMachine)
	}
}

func (s *assignCleanSuite) TestAssignUnitTwiceFails(c *C) {
	s.setupMachines(c)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Assign the first time.
	_, err = s.assignUnit(unit)
	c.Assert(err, IsNil)

	// Check that it fails when called again, even when there's an available machine
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine: unit is already assigned to a machine`))
	c.Assert(m.EnsureDead(), IsNil)
	c.Assert(m.Remove(), IsNil)
}

func (s *assignCleanSuite) TestAssignToMachineNoneAvailable(c *C) {
	// Try to assign a unit to a clean (maybe empty) machine and check that we can't.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	m, err := s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")

	// Add a dying machine and check that it is not chosen.
	m, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = m.Destroy()
	c.Assert(err, IsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")

	// Add a non-unit-hosting machine and check it is not chosen.
	m, err = s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")

	// Add a state management machine which can host units and check it is not chosen.
	m, err = s.State.AddMachine("series", state.JobManageState, state.JobHostUnits)
	c.Assert(err, IsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")

	// Add a environ management machine which can host units and check it is not chosen.
	m, err = s.State.AddMachine("series", state.JobManageEnviron, state.JobHostUnits)
	c.Assert(err, IsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")

	// Add a machine with the wrong series and check it is not chosen.
	m, err = s.State.AddMachine("anotherseries", state.JobHostUnits)
	c.Assert(err, IsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, "all eligible machines in use")
}

func (s *assignCleanSuite) TestAssignUnitWithRemovedService(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Fail if service is removed.
	removeAllUnits(c, s.wordpress)
	err = s.wordpress.Destroy()
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.*: unit not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWithRemovedUnit(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	// Fail if unit is removed.
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	_, err = s.assignUnit(unit)
	c.Assert(err, ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.*: unit not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWorksWithMachine0(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, "0")
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	assignedTo, err := s.assignUnit(unit)
	c.Assert(err, IsNil)
	c.Assert(assignedTo.Id(), Equals, "0")
}

func (s *assignCleanSuite) TestAssignUnitPolicy(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)

	// Check unassigned placements with no clean and/or empty machines.
	for i := 0; i < 10; i++ {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		c.Assert(mid, Equals, strconv.Itoa(1+i))
		assertMachineCount(c, s.State, i+2)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		units, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		c.Assert(units[0].Name(), Equals, unit.Name())
		c.Assert(m.Series(), Equals, "series")
	}

	// Remove units from alternate machines. These machines will still be
	// considered as dirty so will continue to be ignored by the policy.
	for i := 1; i < 11; i += 2 {
		mid := strconv.Itoa(i)
		m, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		units, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		unit := units[0]
		err = unit.UnassignFromMachine()
		c.Assert(err, IsNil)
		err = unit.Destroy()
		c.Assert(err, IsNil)
	}

	var expectedMachines []string
	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	params := state.AddMachineParams{
		ParentId:      hostMachine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(hostMachine.Clean(), IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)
	if s.policy == state.AssignClean {
		expectedMachines = append(expectedMachines, hostMachine.Id())
	}
	expectedMachines = append(expectedMachines, container.Id())

	// Add some more clean machines
	for i := 0; i < 4; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		expectedMachines = append(expectedMachines, m.Id())
	}

	// Assign units to all the expectedMachines machines.
	var got []string
	for _ = range expectedMachines {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		got = append(got, mid)
	}
	sort.Strings(expectedMachines)
	sort.Strings(got)
	c.Assert(got, DeepEquals, expectedMachines)
}

func (s *assignCleanSuite) TestAssignUnitPolicyWithContainers(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)

	// Create a machine and add a new container.
	hostMachine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	params := state.AddMachineParams{
		ParentId:      hostMachine.Id(),
		ContainerType: instance.LXC,
		Series:        "series",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(hostMachine.Clean(), IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, IsNil)

	// Check the first placement goes into the newly created, clean container above.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, IsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, container.Id())

	assertContainerPlacement := func(expectedNumUnits int) {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		c.Assert(mid, Equals, fmt.Sprintf("%d/lxc/0", expectedNumUnits+1))
		assertMachineCount(c, s.State, 2*expectedNumUnits+3)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		units, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		c.Assert(units[0].Name(), Equals, unit.Name())
		c.Assert(m.Series(), Equals, "series")
	}

	// Check unassigned placements with no clean and/or empty machines cause a new container to be created.
	assertContainerPlacement(1)
	assertContainerPlacement(2)

	// Create a new, clean instance and check that the next container creation uses it.
	hostMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, IsNil)
	mid, err = unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, hostMachine.Id()+"/lxc/0")
}

func (s *assignCleanSuite) TestAssignUnitPolicyConcurrently(c *C) {
	_, err := s.State.AddMachine("series", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, IsNil)
	us := make([]*state.Unit, 50)
	for i := range us {
		us[i], err = s.wordpress.AddUnit()
		c.Assert(err, IsNil)
	}
	type result struct {
		u   *state.Unit
		err error
	}
	done := make(chan result)
	for i, u := range us {
		i, u := i, u
		go func() {
			// Start the AssignUnit at different times
			// to increase the likeliness of a race.
			time.Sleep(time.Duration(i) * time.Millisecond / 2)
			err := s.State.AssignUnit(u, s.policy)
			done <- result{u, err}
		}()
	}
	assignments := make(map[string][]*state.Unit)
	for _ = range us {
		r := <-done
		if !c.Check(r.err, IsNil) {
			continue
		}
		id, err := r.u.AssignedMachineId()
		c.Assert(err, IsNil)
		assignments[id] = append(assignments[id], r.u)
	}
	for id, us := range assignments {
		if len(us) != 1 {
			c.Errorf("machine %s expected one unit, got %q", id, us)
		}
	}
	c.Assert(assignments, HasLen, len(us))
}
