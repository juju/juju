package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
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
	// BUG(aram): use error strings from state.
	err = s.unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to machine 2: .*`)

	machineId, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 1)
}

func (s *AssignSuite) TestAssignedMachineIdWhenNotAlive(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	err = s.unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	subCharm := s.AddTestingCharm(c, "logging")
	subSvc, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)

	subUnit, err := subSvc.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)

	testWhenDying(c, s.unit, "", "",
		func() error {
			_, err = s.unit.AssignedMachineId()
			return err
		},
		func() error {
			_, err = subUnit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err := s.unit.Die()
	c.Assert(err, IsNil)
	err = s.service.RemoveUnit(s.unit)
	c.Assert(err, IsNil)

	err = s.unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)

	err = s.service.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)

	err = s.unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)
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

	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = log1Unit.AssignToMachine(machine)
	c.Assert(err, ErrorMatches, ".*: unit is subordinate")
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	id, err := log1Unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Check(id, Equals, machine.Id())
	id, err = log2Unit.AssignedMachineId()
	c.Check(id, Equals, machine.Id())

	// Check that unassigning the principal unassigns the
	// subordinates too.
	err = s.unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	_, err = log1Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging1/0": unit not assigned to machine`)
	_, err = log2Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging2/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	const errPat = ".*: machine or unit dead, or already assigned to machine"
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	testWhenDying(c, unit, errPat, errPat, func() error {
		return unit.AssignToMachine(machine)
	})

	unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	testWhenDying(c, machine, errPat, errPat, func() error {
		return unit.AssignToMachine(machine)
	})

	// Check that UnassignFromMachine works when the unit is dead.
	machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	err = unit.Die()
	c.Assert(err, IsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	// Check that UnassignFromMachine works when the machine is
	// dead.
	machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	err = machine.Die()
	c.Assert(err, IsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)
}
