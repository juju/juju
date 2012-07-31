package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type AssignSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = Suite(&AssignSuite{})

func (s *AssignSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	// Create root machine that shouldn't be used unless requested explicitly.
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *C) {
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	err := s.unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	// Check that the unit has no machine assigned.
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignUnitToMachineAgainFails(c *C) {
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	machineTwo, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	err = s.unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to the same machine should return no error.
	err = s.unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to a different machine should fail.
	err = s.unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to machine 2: unit already assigned to machine 1`)

	machineId, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 1)
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err := s.service.RemoveUnit(s.unit)
	c.Assert(err, IsNil)

	err = s.unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: environment state has changed`)
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": environment state has changed`)

	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)

	err = s.unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: environment state has changed`)
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": environment state has changed`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachine(c *C) {
	// Check that a unit can be assigned to an unused machine.
	origMachine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.unit.AssignToMachine(origMachine)
	c.Assert(err, IsNil)
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)

	// The machine is now unused again, check it's reused on next assignment.
	newService, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	newUnit, err := newService.AddUnit()
	c.Assert(err, IsNil)
	reusedMachine, err := newUnit.AssignToUnusedMachine()
	c.Assert(err, IsNil)
	c.Assert(origMachine.Id(), Equals, reusedMachine.Id())
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineWithChangingService(c *C) {
	// Check for a 'state changed' error if a service is manipulated
	// during reuse.
	err := s.State.RemoveService(s.service)
	c.Assert(err, IsNil)

	_, err = s.unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to unused machine: environment state has changed`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineWithChangingUnit(c *C) {
	// Check for a 'state changed' error if a unit is manipulated
	// during reuse.
	err := s.service.RemoveUnit(s.unit)
	c.Assert(err, IsNil)

	_, err = s.unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to unused machine: environment state has changed`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineOnlyZero(c *C) {
	// Check that the unit can't be assigned to machine zero.
	_, err := s.unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineNoneAvailable(c *C) {
	// Check that assigning without unused machine fails.
	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.unit.AssignToMachine(m1)
	c.Assert(err, IsNil)

	newUnit, err := s.service.AddUnit()
	c.Assert(err, IsNil)

	_, err = newUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *C) {
	// Check that assigning a principal unit assigns its subordinates too.
	subCharm := s.AddTestingCharm(c, "logging")
	logService1, err := s.State.AddService("logging1", subCharm)
	c.Assert(err, IsNil)
	logService2, err := s.State.AddService("logging2", subCharm)
	c.Assert(err, IsNil)
	log1Unit, err := logService1.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)
	log2Unit, err := logService2.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.unit.AssignToMachine(m1)
	c.Assert(err, IsNil)

	id, err := log1Unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Check(id, Equals, m1.Id())
	id, err = log2Unit.AssignedMachineId()
	c.Check(id, Equals, m1.Id())

	// Check that unassigning the principal unassigns the
	// subordinates too.
	err = s.unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	_, err = log1Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging1/0": unit not assigned to machine`)
	_, err = log2Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging2/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignUnit(c *C) {
	// Check nonsensical policy
	fail := func() { s.State.AssignUnit(s.unit, state.AssignmentPolicy("random")) }
	c.Assert(fail, PanicMatches, `unknown unit assignment policy: "random"`)
	_, err := s.unit.AssignedMachineId()
	c.Assert(err, NotNil)
	s.AssertMachineCount(c, 1)

	// Check local placement
	err = s.State.AssignUnit(s.unit, state.AssignLocal)
	c.Assert(err, IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 0)
	s.AssertMachineCount(c, 1)

	// Check unassigned placement with no unused machines
	unit1, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit1, state.AssignUnused)
	c.Assert(err, IsNil)
	mid, err = unit1.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 1)
	s.AssertMachineCount(c, 2)

	// Check unassigned placement on an unused machine
	_, err = s.State.AddMachine()
	unit2, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit2, state.AssignUnused)
	c.Assert(err, IsNil)
	mid, err = unit2.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 2)
	s.AssertMachineCount(c, 3)

	// Check cannot assign subordinates to machines
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	unit3, err := logging.AddUnitSubordinateTo(unit2)
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit3, state.AssignUnused)
	c.Assert(err, ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
}
