// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
)

type AssignSuite struct {
	ConnSuite
	wordpress *state.Service
}

var _ = gc.Suite(&AssignSuite{})

func (s *AssignSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingServiceWithNetworks(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
		[]string{"net1", "net2"},
	)
	wordpress.SetConstraints(constraints.MustParse("networks=net3,^net4,^net5"))
	s.wordpress = wordpress
}

func (s *AssignSuite) addSubordinate(c *gc.C, principal *state.Unit) *state.Unit {
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
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
	unit, err := s.wordpress.AddUnit()
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
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machineTwo, err := s.State.AddMachine("quantal", state.JobHostUnits)
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
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	testWhenDying(c, unit, noErr, noErr,
		func() error {
			_, err = unit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestAssignedMachineIdWhenPrincipalNotAlive(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	subUnit := s.addSubordinate(c, unit)
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := subUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, gc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)

	err = s.wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, gc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *gc.C) {
	// Check that assigning a principal unit assigns its subordinates too.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	// None of the direct unit assign methods work on subordinates.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = subUnit.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to machine 0: unit is a subordinate`)
	_, err = subUnit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to clean machine: unit is a subordinate`)
	_, err = subUnit.AssignToCleanEmptyMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to clean, empty machine: unit is a subordinate`)
	err = subUnit.AssignToNewMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "logging/0" to new machine: unit is a subordinate`)

	// Subordinates know the machine they're indirectly assigned to.
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	id, err := subUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, gc.Equals, machine.Id())

	// Unassigning the principal unassigns the subordinates too.
	err = unit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	_, err = subUnit.AssignedMachineId()
	c.Assert(err, gc.ErrorMatches, `unit "logging/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestDeployerTag(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	principal, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	subordinate := s.addSubordinate(c, principal)

	assertDeployer := func(u *state.Unit, d state.Entity) {
		err := u.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		name, ok := u.DeployerTag()
		if d == nil {
			c.Assert(ok, jc.IsFalse)
		} else {
			c.Assert(ok, jc.IsTrue)
			c.Assert(name, gc.Equals, d.Tag())
		}
	}
	assertDeployer(subordinate, principal)
	assertDeployer(principal, nil)

	err = principal.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	assertDeployer(subordinate, principal)
	assertDeployer(principal, machine)

	err = principal.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	assertDeployer(subordinate, principal)
	assertDeployer(principal, nil)
}

func (s *AssignSuite) TestDirectAssignIgnoresConstraints(c *gc.C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	econs := constraints.MustParse("mem=4G cpu-cores=2")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Machine will take environment constraints on creation.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Unit will take combined service/environ constraints on creation.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Machine keeps its original constraints on direct assignment.
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, econs)
}

func (s *AssignSuite) TestAssignBadSeries(c *gc.C) {
	machine, err := s.State.AddMachine("burble", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to machine 0: series does not match`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)
	assignTest := func() error {
		err := unit.AssignToMachine(machine)
		c.Assert(unit.UnassignFromMachine(), gc.IsNil)
		if subUnit != nil {
			err := subUnit.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
			err = subUnit.Remove()
			c.Assert(err, jc.ErrorIsNil)
			subUnit = nil
		}
		return err
	}
	expect := ".*: unit is not alive"
	testWhenDying(c, unit, expect, expect, assignTest)

	expect = ".*: machine is not alive"
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	testWhenDying(c, machine, expect, expect, assignTest)
}

func (s *AssignSuite) TestAssignMachinePrincipalsChange(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	checkPrincipals := func() []string {
		docID := state.DocID(s.State, machine.Id())
		doc := make(map[string][]string)
		s.machines.FindId(docID).One(&doc)
		principals, ok := doc["principals"]
		if !ok {
			c.Errorf(`machine document does not have a "principals" field`)
		}
		return principals
	}
	c.Assert(checkPrincipals(), gc.DeepEquals, []string{"wordpress/0", "wordpress/1"})

	err = subUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(checkPrincipals(), gc.DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) assertAssignedUnit(c *gc.C, unit *state.Unit) string {
	// Get service networks.
	service, err := unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	serviceNetworks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	serviceCons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the principal is set on the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	machineCons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	machineNetworks, err := machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineNetworks, gc.DeepEquals, serviceNetworks)
	c.Assert(serviceCons.IncludeNetworks(), gc.DeepEquals, machineCons.IncludeNetworks())
	c.Assert(serviceCons.ExcludeNetworks(), gc.DeepEquals, machineCons.ExcludeNetworks())
	machineUnits, err := machine.Units()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineUnits, gc.HasLen, 1)
	// Make sure it is the right unit.
	c.Assert(machineUnits[0].Name(), gc.Equals, unit.Name())
	return machineId
}

func (s *AssignSuite) TestAssignUnitToNewMachine(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignedUnit(c, unit)
}

func (s *AssignSuite) assertAssignUnitToNewMachineContainerConstraint(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	machineId := s.assertAssignedUnit(c, unit)
	c.Assert(state.ParentId(machineId), gc.Not(gc.Equals), "")
	c.Assert(state.ContainerTypeFromId(machineId), gc.Equals, instance.LXC)
}

func (s *AssignSuite) TestAssignUnitToNewMachineContainerConstraint(c *gc.C) {
	// Set up service constraints.
	scons := constraints.MustParse("container=lxc")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitToNewMachineDefaultContainerConstraint(c *gc.C) {
	// Set up env constraints.
	econs := constraints.MustParse("container=lxc")
	err := s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignToNewMachineMakesDirty(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine()
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
	econs := constraints.MustParse("mem=4G cpu-cores=2")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Unit will take combined service/environ constraints on creation.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Change service/env constraints before assigning, to verify this.
	scons = constraints.MustParse("mem=6G cpu-power=800")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	econs = constraints.MustParse("cpu-cores=4")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// The new machine takes the original combined unit constraints.
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	expect := constraints.MustParse("mem=2G cpu-cores=2 cpu-power=400")
	c.Assert(mcons, gc.DeepEquals, expect)
}

func (s *AssignSuite) TestAssignUnitToNewMachineCleanAvailable(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Add a clean machine.
	clean, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToNewMachine()
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
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Make the unit assigned
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	// Try to assign it again
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is already assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitNotAlive(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	// Try to assign a dying unit...
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not alive`)

	// ...and a dead one.
	err = subUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not alive`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitRemoved(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit not found`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesDirty(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Create some units and a clean machine.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	anotherUnit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	makeDirty := txn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(machine), gc.IsNil) },
	}
	defer state.SetTestHooks(c, s.State, makeDirty).Check()

	err = anotherUnit.AssignToNewMachineOrContainer()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "1")

	mid, err = anotherUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "2/lxc/0")
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesHost(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Create a unit and a clean machine.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	addContainer := txn.TestHook{
		Before: func() {
			_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
				Series: "quantal",
				Jobs:   []state.MachineJob{state.JobHostUnits},
			}, machine.Id(), instance.LXC)
			c.Assert(err, jc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, addContainer).Check()

	err = unit.AssignToNewMachineOrContainer()
	c.Assert(err, jc.ErrorIsNil)

	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, "2/lxc/0")
}

func (s *AssignSuite) TestAssignUnitBadPolicy(c *gc.C) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Check nonsensical policy
	err = s.State.AssignUnit(unit, state.AssignmentPolicy("random"))
	c.Assert(err, gc.ErrorMatches, `.*unknown unit assignment policy: "random"`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, gc.NotNil)
	assertMachineCount(c, s.State, 0)
}

func (s *AssignSuite) TestAssignUnitLocalPolicy(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobManageEnviron, state.JobHostUnits) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		err = s.State.AssignUnit(unit, state.AssignLocal)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mid, gc.Equals, m.Id())
		assertMachineCount(c, s.State, 1)
	}
}

func (s *AssignSuite) assertAssignUnitNewPolicyNoContainer(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits) // available machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineCount(c, s.State, 2)
	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.ParentId(id), gc.Equals, "")
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
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineCount(c, s.State, 3)
	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, "1/lxc/0")
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraint(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Set up service constraints.
	scons := constraints.MustParse("container=lxc")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithDefaultContainerConstraint(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// Set up env constraints.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitWithSubordinate(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Check cannot assign subordinates to machines
	subUnit := s.addSubordinate(c, unit)
	for _, policy := range []state.AssignmentPolicy{
		state.AssignLocal, state.AssignNew, state.AssignClean, state.AssignCleanEmpty,
	} {
		err = s.State.AssignUnit(subUnit, policy)
		c.Assert(err, gc.ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
	}
}

func assertMachineCount(c *gc.C, st *state.State, expect int) {
	ms, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.HasLen, expect, gc.Commentf("%v", ms))
}

// assignCleanSuite has tests for assigning units to 1. clean, and 2. clean&empty machines.
type assignCleanSuite struct {
	ConnSuite
	policy    state.AssignmentPolicy
	wordpress *state.Service
}

var _ = gc.Suite(&assignCleanSuite{ConnSuite{}, state.AssignCleanEmpty, nil})
var _ = gc.Suite(&assignCleanSuite{ConnSuite{}, state.AssignClean, nil})

func (s *assignCleanSuite) SetUpTest(c *gc.C) {
	c.Logf("assignment policy for this test: %q", s.policy)
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.wordpress = wordpress
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
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

func (s *assignCleanSuite) assertMachineEmpty(c *gc.C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(containers), gc.Equals, 0)
}

func (s *assignCleanSuite) assertMachineNotEmpty(c *gc.C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(containers), gc.Not(gc.Equals), 0)
}

// setupMachines creates a combination of machines with which to test.
func (s *assignCleanSuite) setupMachines(c *gc.C) (hostMachine *state.Machine, container *state.Machine, cleanEmptyMachine *state.Machine) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Add some units to another service and allocate them to machines
	service1 := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		u, err := service1.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		err = u.AssignToMachine(m)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = u
	}

	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXC)
	c.Assert(hostMachine.Clean(), jc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	// Create a new, clean, empty machine.
	cleanEmptyMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanEmptyMachine.Clean(), jc.IsTrue)
	s.assertMachineEmpty(c, cleanEmptyMachine)
	return hostMachine, container, cleanEmptyMachine
}

func (s *assignCleanSuite) assertAssignUnit(c *gc.C, expectedMachine *state.Machine) {
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	reusedMachine, err := s.assignUnit(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reusedMachine.Id(), gc.Equals, expectedMachine.Id())
	c.Assert(reusedMachine.Clean(), jc.IsFalse)
}

func (s *assignCleanSuite) TestAssignUnit(c *gc.C) {
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

func (s *assignCleanSuite) TestAssignUnitTwiceFails(c *gc.C) {
	s.setupMachines(c)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Assign the first time.
	_, err = s.assignUnit(unit)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it fails when called again, even when there's an available machine
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, gc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine: unit is already assigned to a machine`))
	c.Assert(m.EnsureDead(), gc.IsNil)
	c.Assert(m.Remove(), gc.IsNil)
}

const eligibleMachinesInUse = "all eligible machines in use"

func (s *assignCleanSuite) TestAssignToMachineNoneAvailable(c *gc.C) {
	// Try to assign a unit to a clean (maybe empty) machine and check that we can't.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.assignUnit(unit)
	c.Assert(m, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)

	// Add a state management machine which can host units and check it is not chosen.
	// Note that this must the first machine added, as AddMachine can only
	// be used to add state-manager machines for the bootstrap machine.
	m, err = s.State.AddMachine("quantal", state.JobManageEnviron, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)

	// Add a dying machine and check that it is not chosen.
	m, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)

	// Add two environ manager machines and check they are not chosen.
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes.Added, gc.HasLen, 3)

	m, err = s.assignUnit(unit)
	c.Assert(m, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)

	// Add a machine with the wrong series and check it is not chosen.
	m, err = s.State.AddMachine("anotherseries", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)
}

var assignUsingConstraintsTests = []struct {
	unitConstraints         string
	hardwareCharacteristics string
	assignOk                bool
}{
	{
		unitConstraints:         "",
		hardwareCharacteristics: "",
		assignOk:                true,
	}, {
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "none",
		assignOk:                false,
	}, {
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "cpu-cores=1",
		assignOk:                false,
	}, {
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "arch=amd64",
		assignOk:                true,
	}, {
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "arch=i386",
		assignOk:                false,
	}, {
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "none",
		assignOk:                false,
	}, {
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "cpu-cores=1",
		assignOk:                false,
	}, {
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "mem=4G",
		assignOk:                true,
	}, {
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "mem=2G",
		assignOk:                false,
	}, {
		unitConstraints:         "cpu-cores=2",
		hardwareCharacteristics: "cpu-cores=2",
		assignOk:                true,
	}, {
		unitConstraints:         "cpu-cores=2",
		hardwareCharacteristics: "cpu-cores=1",
		assignOk:                false,
	}, {
		unitConstraints:         "cpu-cores=2",
		hardwareCharacteristics: "mem=4G",
		assignOk:                false,
	}, {
		unitConstraints:         "cpu-power=50",
		hardwareCharacteristics: "cpu-power=50",
		assignOk:                true,
	}, {
		unitConstraints:         "cpu-power=100",
		hardwareCharacteristics: "cpu-power=50",
		assignOk:                false,
	}, {
		unitConstraints:         "cpu-power=50",
		hardwareCharacteristics: "mem=4G",
		assignOk:                false,
	}, {
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "cpu-power=50",
		assignOk:                false,
	}, {
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "root-disk=4096",
		assignOk:                false,
	}, {
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "root-disk=8192",
		assignOk:                true,
	}, {
		unitConstraints:         "arch=amd64 mem=4G cpu-cores=2 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 mem=8G cpu-cores=2 root-disk=8192 cpu-power=50",
		assignOk:                true,
	}, {
		unitConstraints:         "arch=amd64 mem=4G cpu-cores=2 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 mem=8G cpu-cores=1 root-disk=4096 cpu-power=50",
		assignOk:                false,
	},
}

func (s *assignCleanSuite) TestAssignUsingConstraintsToMachine(c *gc.C) {
	for i, t := range assignUsingConstraintsTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.unitConstraints)
		err := s.State.SetEnvironConstraints(cons)
		c.Assert(err, jc.ErrorIsNil)

		unit, err := s.wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)

		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		if t.hardwareCharacteristics != "none" {
			hc := instance.MustParseHardware(t.hardwareCharacteristics)
			err = m.SetProvisioned("inst-id", "fake_nonce", &hc)
			c.Assert(err, jc.ErrorIsNil)
		}

		um, err := s.assignUnit(unit)
		if t.assignOk {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(um.Id(), gc.Equals, m.Id())
		} else {
			c.Assert(um, gc.IsNil)
			c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)
			// Destroy the machine so it can't be used for the next test.
			err = m.Destroy()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *assignCleanSuite) TestAssignUnitWithRemovedService(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Fail if service is removed.
	removeAllUnits(c, s.wordpress)
	err = s.wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, gc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.* not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWithRemovedUnit(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	// Fail if unit is removed.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.assignUnit(unit)
	c.Assert(err, gc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.*: unit not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWorksWithMachine0(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	assignedTo, err := s.assignUnit(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedTo.Id(), gc.Equals, "0")
}

func (s *assignCleanSuite) setupSingleStorage(c *gc.C, kind, pool string) (*state.Service, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return service, unit, storageTag
}

func (s *assignCleanSuite) TestAssignToMachine(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	filesystemAttachments, err := s.State.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
}

func (s *assignCleanSuite) TestAssignToMachineErrors(c *gc.C) {
	registry.RegisterProvider("static", &dummy.StorageProvider{
		IsDynamic: false,
	})
	registry.RegisterEnvironStorageProviders("someprovider", "static")
	defer registry.RegisterProvider("static", nil)

	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(
		err, gc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: "static" storage provider does not support dynamic storage`,
	)

	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(container)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "storage-filesystem/0" to machine 0/lxc/0: adding storage to lxc container not supported`)
}

func (s *assignCleanSuite) TestAssignUnitWithNonDynamicStorageCleanAvailable(c *gc.C) {
	registry.RegisterProvider("static", &dummy.StorageProvider{
		IsDynamic: false,
	})
	registry.RegisterEnvironStorageProviders("someprovider", "static")
	defer registry.RegisterProvider("static", nil)
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, gc.Not(gc.Equals), clean.Id())
}

func (s *assignCleanSuite) TestAssignUnitWithDynamicStorageCleanAvailable(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")
	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, gc.Equals, clean.Id())

	// Check that a volume attachments were added to the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	volumeAttachments, err := s.State.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)

	volume, err := s.State.Volume(volumeAttachments[0].Volume())
	c.Assert(err, jc.ErrorIsNil)
	volumeStorageInstance, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeStorageInstance, gc.Equals, storageAttachments[0].StorageInstance())
}

func (s *assignCleanSuite) TestAssignUnitPolicy(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Check unassigned placements with no clean and/or empty machines.
	for i := 0; i < 10; i++ {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mid, gc.Equals, strconv.Itoa(1+i))
		assertMachineCount(c, s.State, i+2)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.HasLen, 1)
		c.Assert(units[0].Name(), gc.Equals, unit.Name())
		c.Assert(m.Series(), gc.Equals, "quantal")
	}

	// Remove units from alternate machines. These machines will still be
	// considered as dirty so will continue to be ignored by the policy.
	for i := 1; i < 11; i += 2 {
		mid := strconv.Itoa(i)
		m, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.HasLen, 1)
		unit := units[0]
		err = unit.UnassignFromMachine()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	var expectedMachines []string
	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXC)
	c.Assert(hostMachine.Clean(), jc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)
	if s.policy == state.AssignClean {
		expectedMachines = append(expectedMachines, hostMachine.Id())
	}
	expectedMachines = append(expectedMachines, container.Id())

	// Add some more clean machines
	for i := 0; i < 4; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		expectedMachines = append(expectedMachines, m.Id())
	}

	// Assign units to all the expectedMachines machines.
	var got []string
	for _ = range expectedMachines {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		got = append(got, mid)
	}
	sort.Strings(expectedMachines)
	sort.Strings(got)
	c.Assert(got, gc.DeepEquals, expectedMachines)
}

func (s *assignCleanSuite) TestAssignUnitPolicyWithContainers(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)

	// Create a machine and add a new container.
	hostMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXC)
	err = hostMachine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostMachine.Clean(), jc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxc")
	err = s.State.SetEnvironConstraints(econs)
	c.Assert(err, jc.ErrorIsNil)

	// Check the first placement goes into the newly created, clean container above.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, jc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, container.Id())

	assertContainerPlacement := func(expectedNumUnits int) {
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mid, gc.Equals, fmt.Sprintf("%d/lxc/0", expectedNumUnits+1))
		assertMachineCount(c, s.State, 2*expectedNumUnits+3)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.HasLen, 1)
		c.Assert(units[0].Name(), gc.Equals, unit.Name())
		c.Assert(m.Series(), gc.Equals, "quantal")
	}

	// Check unassigned placements with no clean and/or empty machines cause a new container to be created.
	assertContainerPlacement(1)
	assertContainerPlacement(2)

	// Create a new, clean instance and check that the next container creation uses it.
	hostMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	unit, err = s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, jc.ErrorIsNil)
	mid, err = unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, hostMachine.Id()+"/lxc/0")
}

func (s *assignCleanSuite) TestAssignUnitPolicyConcurrently(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron) // bootstrap machine
	c.Assert(err, jc.ErrorIsNil)
	us := make([]*state.Unit, 50)
	for i := range us {
		us[i], err = s.wordpress.AddUnit()
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
			err := s.State.AssignUnit(u, s.policy)
			done <- result{u, err}
		}()
	}
	assignments := make(map[string][]*state.Unit)
	for _ = range us {
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
