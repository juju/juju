package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
)

type AssignSuite struct {
	UtilSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = Suite(&AssignSuite{})

func (s *AssignSuite) SetUpTest(c *C) {
	s.UtilSuite.SetUpTest(c)
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
	c.Assert(err, ErrorMatches, `can't get machine id of unit "wordpress/0": unit not assigned to machine`)
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
	// BUG: use error strings from state.
	err = s.unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `can't assign unit "wordpress/0" to machine 2: .*`)

	machineId, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 1)
}
