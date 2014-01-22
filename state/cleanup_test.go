package state_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
)

type CleanupSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CleanupSuite{})

func (s *CleanupSuite) TestCleanupDyingServiceUnits(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a service with some units.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit()
		c.Assert(err, gc.IsNil)
		units[i] = unit
	}
	preventUnitDestroyRemove(c, units[0])
	s.assertDoesNotNeedCleanup(c)

	// Destroy the service and check the units are unaffected, but a cleanup
	// has been scheduled.
	err := mysql.Destroy()
	c.Assert(err, gc.IsNil)
	for _, unit := range units {
		err := unit.Refresh()
		c.Assert(err, gc.IsNil)
	}
	s.assertNeedsCleanup(c)

	// Run the cleanup, and check that units are all destroyed as appropriate.
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
	err = units[0].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(units[0].Life(), gc.Equals, state.Dying)
	err = units[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = units[2].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *CleanupSuite) TestCleanupEnvironmentServices(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a service with some units.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit()
		c.Assert(err, gc.IsNil)
		units[i] = unit
	}
	s.assertDoesNotNeedCleanup(c)

	// Destroy the environment and check the service and units are
	// unaffected, but a cleanup for the service has been scheduled.
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	err = env.Destroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	err = mysql.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(mysql.Life(), gc.Equals, state.Dying)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, gc.IsNil)
		c.Assert(unit.Life(), gc.Equals, state.Alive)
	}

	// The first cleanup Destroys the service, which
	// schedules another cleanup to destroy the units.
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	}
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupRelationSettings(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a relation with a unit in scope.
	pr := NewPeerRelation(c, s.State)
	rel := pr.ru0.Relation()
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, gc.IsNil)
	s.assertDoesNotNeedCleanup(c)

	// Destroy the service, check the relation's still around.
	err = pr.svc.Destroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	err = rel.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)
	s.assertDoesNotNeedCleanup(c)

	// The unit leaves scope, triggering relation removal.
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)

	// Settings are not destroyed yet...
	settings, err := pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})

	// ...but they are on cleanup.
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
}

func (s *CleanupSuite) TestForceDestroyMachineErrors(c *gc.C) {
	manager, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	s.assertDoesNotNeedCleanup(c)
	err = manager.ForceDestroy()
	expect := fmt.Sprintf("machine %s is required by the environment", manager.Id())
	c.Assert(err, gc.ErrorMatches, expect)
	s.assertDoesNotNeedCleanup(c)
	assertLife(c, manager, state.Alive)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineUnit(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a machine.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// Create a relation with a unit in scope and assigned to the machine.
	pr := NewPeerRelation(c, s.State)
	err = pr.u0.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertDoesNotNeedCleanup(c)

	// Force machine destruction, check cleanup queued.
	err = machine.ForceDestroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the unit has been removed...
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
	assertRemoved(c, pr.u0)

	// ...and the unit has departed relation scope...
	assertNotInScope(c, pr.ru0)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineWithContainer(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a machine with a container.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)

	// Create active units (in relation scope, with subordinates).
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	err = prr.pru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// Assign the various units to machines.
	err = prr.pu0.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	err = prr.pu1.AssignToMachine(container)
	c.Assert(err, gc.IsNil)
	s.assertDoesNotNeedCleanup(c)

	// Force removal of the top-level machine.
	err = machine.ForceDestroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)

	// And do it again, just to check that the second cleanup doc for the same
	// machine doesn't cause problems down the line.
	err = machine.ForceDestroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the container has been removed...
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	// ...and so have all the units...
	assertRemoved(c, prr.pu0)
	assertRemoved(c, prr.pu1)
	assertRemoved(c, prr.ru0)
	assertRemoved(c, prr.ru1)

	// ...and none of the units have left relation scopes occupied...
	assertNotInScope(c, prr.pru0)
	assertNotInScope(c, prr.pru1)
	assertNotInScope(c, prr.rru0)
	assertNotInScope(c, prr.rru1)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
}

func (s *CleanupSuite) TestNothingToCleanup(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) assertCleanupRuns(c *gc.C) {
	err := s.State.Cleanup()
	c.Assert(err, gc.IsNil)
}

func (s *CleanupSuite) assertNeedsCleanup(c *gc.C) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(actual, jc.IsTrue)
}

func (s *CleanupSuite) assertDoesNotNeedCleanup(c *gc.C) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(actual, jc.IsFalse)
}
