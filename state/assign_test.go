// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/state"
)

type AssignSuite struct {
	ConnSuite
	wordpress *state.Application
}

var _ = gc.Suite(&AssignSuite{})

func (s *AssignSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
	)
	s.wordpress = wordpress
}

func (s *AssignSuite) addSubordinate(c *gc.C, principal *state.Unit) *state.Unit {
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(principal)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	return subUnit
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	err = unit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the unit has no machine assigned.
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToMachineAgainFails(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machineTwo, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, jc.ErrorIsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, jc.ErrorIsNil)

	// Assigning the unit to a different machine should fail.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to machine 1: unit is already assigned to a machine`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineId, gc.Equals, "0")
}

func (s *AssignSuite) TestAssignedMachineIdWhenNotAlive(c *gc.C) {
	store := state.NewObjectStore(c, s.State.ModelUUID())

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	testWhenDying(c, store, unit, noErr, noErr,
		func() error {
			_, err = unit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestAssignedMachineIdWhenPrincipalNotAlive(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	subUnit := s.addSubordinate(c, unit)
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	mid, err := subUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, gc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)

	err = s.wordpress.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, gc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *gc.C) {
	// Check that assigning a principal unit assigns its subordinates too.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Units need to be assigned to a machine before the subordinates
	// are created in order for the subordinate to get the machine ID.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	subUnit := s.addSubordinate(c, unit)

	// None of the direct unit assign methods work on subordinates.
	err = subUnit.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to machine 0: unit is a subordinate`)
	err = subUnit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to new machine: unit is a subordinate`)

	// Subordinates know the machine they're indirectly assigned to.
	id, err := subUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, gc.Equals, machine.Id())
}

func (s *AssignSuite) TestDirectAssignIgnoresConstraints(c *gc.C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	econs := constraints.MustParse("mem=4G cores=2")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Machine will take model constraints on creation.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Unit will take combined application/model constraints on creation.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Machine keeps its original constraints on direct assignment.
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, econs)
}

func (s *AssignSuite) TestAssignBadSeries(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("22.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to machine 0: base does not match.*`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *gc.C) {
	store := state.NewObjectStore(c, s.State.ModelUUID())

	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)
	assignTest := func() error {
		err := unit.AssignToMachine(machine)
		c.Assert(unit.UnassignFromMachine(), gc.IsNil)
		if subUnit != nil {
			err := subUnit.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
			err = subUnit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
			c.Assert(err, jc.ErrorIsNil)
			subUnit = nil
		}
		return err
	}
	expect := ".*: unit is not found or not alive"
	testWhenDying(c, store, unit, expect, expect, assignTest)

	expect = ".*: machine is not found or not alive"
	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	testWhenDying(c, store, machine, expect, expect, assignTest)
}

func (s *AssignSuite) TestAssignMachineDifferentSeries(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches,
		`cannot assign unit "wordpress/0" to machine 0: base does not match.*`)
}

func (s *AssignSuite) TestPrincipals(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	principals := machine.Principals()
	c.Assert(principals, jc.DeepEquals, []string{})

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	principals = machine.Principals()
	c.Assert(principals, jc.DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) TestAssignMachinePrincipalsChange(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	checkPrincipals := func() []string {
		err := machine.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		return machine.Principals()
	}
	c.Assert(checkPrincipals(), gc.DeepEquals, []string{"wordpress/0", "wordpress/1"})

	err = subUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(checkPrincipals(), gc.DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) assertAssignedUnit(c *gc.C, unit *state.Unit) string {
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the principal is set on the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	machineUnits, err := machine.Units()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineUnits, gc.HasLen, 1)
	// Make sure it is the right unit.
	c.Assert(machineUnits[0].Name(), gc.Equals, unit.Name())
	return machineId
}

func (s *AssignSuite) TestAssignUnitToNewMachine(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignedUnit(c, unit)
}

func (s *AssignSuite) assertAssignUnitToNewMachineContainerConstraint(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	machineId := s.assertAssignedUnit(c, unit)
	c.Assert(container.ParentId(machineId), gc.Not(gc.Equals), "")
	c.Assert(container.ContainerTypeFromId(machineId), gc.Equals, instance.LXD)
}

func (s *AssignSuite) TestAssignUnitToNewMachineContainerConstraint(c *gc.C) {
	// Set up application constraints.
	scons := constraints.MustParse("container=lxd")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitToNewMachineDefaultContainerConstraint(c *gc.C) {
	// Set up model constraints.
	econs := constraints.MustParse("container=lxd")
	err := s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignToNewMachineMakesDirty(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Clean(), jc.IsFalse)
}

func (s *AssignSuite) TestAssignUnitToNewMachineSetsConstraints(c *gc.C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	econs := constraints.MustParse("mem=4G cores=2")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Unit will take combined application/model constraints on creation.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Change application/model constraints before assigning, to verify this.
	scons = constraints.MustParse("mem=6G cpu-power=800")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	econs = constraints.MustParse("cores=4")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// The new machine takes the original combined unit constraints.
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	expect := constraints.MustParse("arch=amd64 mem=2G cores=2 cpu-power=400")
	c.Assert(mcons, gc.DeepEquals, expect)
}

func (s *AssignSuite) TestAssignUnitToNewMachineCleanAvailable(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Add a clean machine.
	clean, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Not(gc.Equals), clean.Id())
}

func (s *AssignSuite) TestAssignUnitToNewMachineAlreadyAssigned(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Make the unit assigned
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	// Try to assign it again
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is already assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitNotAlive(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	// Try to assign a dying unit...
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not found or not alive`)

	// ...and a dead one.
	err = subUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not found or not alive`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitRemoved(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit not found`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesDirty(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Create some units and a clean machine.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	anotherUnit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	makeDirty := jujutxn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(machine), gc.IsNil) },
	}
	defer state.SetTestHooks(c, s.State, makeDirty).Check()

	err = anotherUnit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "1")

	mid, err = anotherUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "2/lxd/0")
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesHost(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Create a unit and a clean machine.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	addContainer := jujutxn.TestHook{
		Before: func() {
			_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
				Base: state.UbuntuBase("12.10"),
				Jobs: []state.MachineJob{state.JobHostUnits},
			}, machine.Id(), instance.LXD)
			c.Assert(err, jc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, addContainer).Check()

	err = unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)

	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "2/lxd/0")
}

func (s *AssignSuite) TestAssignUnitBadPolicy(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Check nonsensical policy
	err = s.State.AssignUnit(defaultInstancePrechecker, unit, state.AssignmentPolicy("random"))
	c.Assert(err, gc.ErrorMatches, `.*unknown unit assignment policy: "random"`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.NotNil)
	assertMachineCount(c, s.State, 0)
}

func (s *AssignSuite) TestAssignUnitLocalPolicy(c *gc.C) {
	m, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		err = s.State.AssignUnit(defaultInstancePrechecker, unit, state.AssignLocal)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mid, gc.Equals, m.Id())
		assertMachineCount(c, s.State, 1)
	}
}

func (s *AssignSuite) assertAssignUnitNewPolicyNoContainer(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits) // available machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AssignUnit(defaultInstancePrechecker, unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineCount(c, s.State, 2)
	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(container.ParentId(id), gc.Equals, "")
}

func (s *AssignSuite) TestAssignUnitNewPolicy(c *gc.C) {
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraintIgnoresNone(c *gc.C) {
	scons := constraints.MustParse("container=none")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) assertAssignUnitNewPolicyWithContainerConstraint(c *gc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(defaultInstancePrechecker, unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineCount(c, s.State, 3)
	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, "1/lxd/0")
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraint(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Set up application constraints.
	scons := constraints.MustParse("container=lxd")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithDefaultContainerConstraint(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Set up model constraints.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitWithSubordinate(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Check cannot assign subordinates to machines
	subUnit := s.addSubordinate(c, unit)
	for _, policy := range []state.AssignmentPolicy{
		state.AssignLocal, state.AssignNew,
	} {
		err = s.State.AssignUnit(defaultInstancePrechecker, subUnit, policy)
		c.Assert(err, gc.ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
	}
}

func assertMachineCount(c *gc.C, st *state.State, expect int) {
	ms, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.HasLen, expect, gc.Commentf("%v", ms))
}

// assignSuite has tests for assigning units to 1. clean, and 2. clean&empty machines.
type assignSuite struct {
	ConnSuite
	wordpress *state.Application
}

var _ = gc.Suite(&assignSuite{ConnSuite: ConnSuite{}, wordpress: nil})

func (s *assignSuite) SetUpTest(c *gc.C) {
	c.Logf("assignment policy for this test: %q", s.policy)
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.wordpress = wordpress
	s.ConnSuite.policy.Providers = map[string]domainstorage.StoragePoolDetails{
		"loop-pool": {Name: "loop-pool", Provider: "loop"},
	}
}

func (s *assignSuite) setupSingleStorage(c *gc.C, kind, pool string) (*state.Application, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return application, unit, storageTag
}

func (s *assignSuite) TestAssignToMachine(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	filesystemAttachments, err := sb.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
}

func (s *assignSuite) TestAssignToMachineErrors(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(
		err, gc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: "static" storage provider does not support dynamic storage`,
	)

	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(container)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "storage-filesystem/0" to machine 0/lxd/0: adding storage to lxd container not supported`)
}

func (s *assignSuite) TestAssignUnitWithNonDynamicStorageAndMachinePlacementDirective(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	placement := &instance.Placement{
		instance.MachineScope, clean.Id(),
	}
	err = s.State.AssignUnitWithPlacement(defaultInstancePrechecker, unit, placement, nil)
	c.Assert(
		err, gc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: "static" storage provider does not support dynamic storage`,
	)
}

func (s *assignSuite) TestAssignUnitWithNonDynamicStorageAndZonePlacementDirective(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	placement := &instance.Placement{
		s.State.ModelUUID(), "zone=test",
	}
	err = s.State.AssignUnitWithPlacement(defaultInstancePrechecker, unit, placement, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, gc.Not(gc.Equals), clean.Id())
}

func (s *assignSuite) TestAssignUnitPolicyConcurrently(c *gc.C) {
	_, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unitCount := 50
	if raceDetector {
		unitCount = 10
	}
	us := make([]*state.Unit, unitCount)
	for i := range us {
		us[i], err = s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
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
			err := s.State.AssignUnit(defaultInstancePrechecker, u, state.AssignNew)
			done <- result{u, err}
		}()
	}
	assignments := make(map[string][]*state.Unit)
	for range us {
		r := <-done
		if !c.Check(r.err, gc.IsNil) {
			continue
		}
		id, err := r.u.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		assignments[id] = append(assignments[id], r.u)
	}
	for id, us := range assignments {
		if len(us) != 1 {
			c.Errorf("machine %s expected one unit, got %q", id, us)
		}
	}
	c.Assert(assignments, gc.HasLen, len(us))
}
