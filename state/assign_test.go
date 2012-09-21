package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"sort"
)

type AssignSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&AssignSuite{})

func (s *AssignSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
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
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignUnitToMachineAgainFails(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	machineTwo, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to a different machine should fail.
	// BUG(aram): use error strings from state.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to machine 1: .*`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 0)
}

func (s *AssignSuite) TestAssignedMachineIdWhenNotAlive(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine()
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
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	subCharm := s.AddTestingCharm(c, "logging")
	subSvc, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)

	subUnit, err := subSvc.AddUnitSubordinateTo(unit)
	c.Assert(err, IsNil)

	testWhenDying(c, unit, noErr, noErr,
		func() error {
			_, err = subUnit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err = unit.Die()
	c.Assert(err, IsNil)
	err = service.RemoveUnit(unit)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)

	err = service.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(service)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "wordpress/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check that assigning a principal unit assigns its subordinates too.
	subCharm := s.AddTestingCharm(c, "logging")
	logService1, err := s.State.AddService("logging1", subCharm)
	c.Assert(err, IsNil)
	logService2, err := s.State.AddService("logging2", subCharm)
	c.Assert(err, IsNil)
	log1Unit, err := logService1.AddUnitSubordinateTo(unit)
	c.Assert(err, IsNil)
	log2Unit, err := logService2.AddUnitSubordinateTo(unit)
	c.Assert(err, IsNil)

	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = log1Unit.AssignToMachine(machine)
	c.Assert(err, ErrorMatches, ".*: unit is a subordinate")
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)

	id, err := log1Unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Check(id, Equals, machine.Id())
	id, err = log2Unit.AssignedMachineId()
	c.Check(id, Equals, machine.Id())

	// Check that unassigning the principal unassigns the
	// subordinates too.
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)
	_, err = log1Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging1/0": unit not assigned to machine`)
	_, err = log2Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `cannot get machine id of unit "logging2/0": unit not assigned to machine`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	const unitDeadErr = ".*: unit is dead"
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	assignTest := func() error {
		err := unit.AssignToMachine(machine)
		err1 := unit.UnassignFromMachine()
		c.Assert(err1, IsNil)
		return err
	}
	testWhenDying(c, unit, unitDeadErr, unitDeadErr, assignTest)

	const machineDeadErr = ".*: machine is dead"
	unit, err = service.AddUnit()
	c.Assert(err, IsNil)
	testWhenDying(c, machine, machineDeadErr, machineDeadErr, assignTest)
}

func (s *AssignSuite) TestUnassignMachineWhenDying(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	// Check that UnassignFromMachine works when the unit is dead.
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
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
	unit, err = service.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	err = machine.Die()
	c.Assert(err, IsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)
}

func (s *AssignSuite) TestAssignMachinePrincipalsChange(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	unit, err = service.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, IsNil)
	subCharm := s.AddTestingCharm(c, "logging")
	logService, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	_, err = logService.AddUnitSubordinateTo(unit)
	c.Assert(err, IsNil)

	doc := make(map[string][]string)
	s.ConnSuite.machines.FindId(machine.Id()).One(&doc)
	principals, ok := doc["principals"]
	if !ok {
		c.Errorf(`machine document does not have a "principals" field`)
	}
	c.Assert(principals, DeepEquals, []string{"wordpress/0", "wordpress/1"})

	err = unit.Die()
	c.Assert(err, IsNil)
	err = service.RemoveUnit(unit)
	c.Assert(err, IsNil)
	doc = make(map[string][]string)
	s.ConnSuite.machines.FindId(machine.Id()).One(&doc)
	principals, ok = doc["principals"]
	if !ok {
		c.Errorf(`machine document does not have a "principals" field`)
	}
	c.Assert(principals, DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) TestAssignUnitToUnusedMachine(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	// Add some units to another service and allocate them to machines
	service1, err := s.State.AddService("wordpress1", s.charm)
	c.Assert(err, IsNil)
	units := make([]*state.Unit, 3)
	for i := range units {
		u, err := service1.AddUnit()
		c.Assert(err, IsNil)
		m, err := s.State.AddMachine()
		c.Assert(err, IsNil)
		err = u.AssignToMachine(m)
		c.Assert(err, IsNil)
		units[i] = u
	}

	// Assign the suite's unit to a machine, then remove the service
	// so the machine becomes available again.
	origMachine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(origMachine)
	c.Assert(err, IsNil)
	err = unit.Die()
	c.Assert(err, IsNil)
	err = service.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(service)
	c.Assert(err, IsNil)

	// Check that AssignToUnusedMachine finds the old (now unused) machine.
	newService, err := s.State.AddService("wordpress2", s.charm)
	c.Assert(err, IsNil)
	newUnit, err := newService.AddUnit()
	c.Assert(err, IsNil)
	reusedMachine, err := newUnit.AssignToUnusedMachine()
	c.Assert(err, IsNil)
	c.Assert(reusedMachine.Id(), Equals, origMachine.Id())

	// Check that it fails when called again, even when there's an available machine
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = newUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress2/0" to unused machine: unit is already assigned to a machine`)
	err = m.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveMachine(m.Id())
	c.Assert(err, IsNil)

	// Try to assign another unit to an unused machine
	// and check that we can't
	newUnit, err = newService.AddUnit()
	c.Assert(err, IsNil)

	m, err = newUnit.AssignToUnusedMachine()
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, `all machines in use`)

	// Add a dying machine and check that it is not chosen.
	m, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.Kill()
	c.Assert(err, IsNil)
	m, err = newUnit.AssignToUnusedMachine()
	c.Assert(m, IsNil)
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineWithRemovedService(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Fail if service is removed.
	err = service.Die()
	c.Assert(err, IsNil)

	err = s.State.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)

	_, err = unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to unused machine.*: cannot get unit "wordpress/0": not found`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineWithRemovedUnit(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Fail if unit is removed.
	err = unit.Die()
	c.Assert(err, IsNil)
	err = service.RemoveUnit(unit)
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)

	_, err = unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `cannot assign unit "wordpress/0" to unused machine.*: cannot get unit "wordpress/0": not found`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineOnlyZero(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check that the unit can't be assigned to machine zero.
	_, err = unit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *AssignSuite) TestAssignUnitToUnusedMachineNoneAvailable(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check that assigning without unused machine fails.
	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(m1)
	c.Assert(err, IsNil)

	newUnit, err := service.AddUnit()
	c.Assert(err, IsNil)

	_, err = newUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *AssignSuite) TestAssignUnitBadPolicy(c *C) {
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	// Check nonsensical policy
	fail := func() { s.State.AssignUnit(unit, state.AssignmentPolicy("random")) }
	c.Assert(fail, PanicMatches, `unknown unit assignment policy: "random"`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, NotNil)
	assertMachineCount(c, s.State, 0)
}

func (s *AssignSuite) TestAssignUnitLocalPolicy(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	for i := 0; i < 2; i++ {
		err = s.State.AssignUnit(unit, state.AssignLocal)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		c.Assert(mid, Equals, 0)
		assertMachineCount(c, s.State, 1)
	}
}

func (s *AssignSuite) TestAssignUnitUnusedPolicy(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	// Check unassigned placements with no unused machines.
	for i := 0; i < 10; i++ {
		unit, err := service.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, state.AssignUnused)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		c.Assert(mid, Equals, 1+i)
		assertMachineCount(c, s.State, i+2)

		// Sanity check that the machine knows about its assigned unit.
		m, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		units, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		c.Assert(units[0].Name(), Equals, unit.Name())
	}

	// Remove units from alternate machines.
	var unused []int
	for mid := 1; mid < 11; mid += 2 {
		m, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		units, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(units, HasLen, 1)
		unit := units[0]
		err = unit.UnassignFromMachine()
		c.Assert(err, IsNil)
		err = unit.Die()
		c.Assert(err, IsNil)
		unused = append(unused, mid)
	}
	// Add some more unused machines
	for i := 0; i < 4; i++ {
		m, err := s.State.AddMachine()
		c.Assert(err, IsNil)
		unused = append(unused, m.Id())
	}

	// Assign units to all the unused machines.
	var got []int
	for _ = range unused {
		unit, err := service.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, state.AssignUnused)
		c.Assert(err, IsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		got = append(got, mid)
	}
	sort.Ints(unused)
	sort.Ints(got)
	c.Assert(got, DeepEquals, unused)
}

func (s *AssignSuite) TestAssignSubordinate(c *C) {
	_, err := s.State.AddMachine() // bootstrap machine
	c.Assert(err, IsNil)
	service, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	// Check cannot assign subordinates to machines
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	unit2, err := logging.AddUnitSubordinateTo(unit)
	c.Assert(err, IsNil)
	err = s.State.AssignUnit(unit2, state.AssignUnused)
	c.Assert(err, ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
}

func assertMachineCount(c *C, st *state.State, expect int) {
	ms, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ms, HasLen, expect, Commentf("%v", ms))
}
