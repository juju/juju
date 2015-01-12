// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
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
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	preventUnitDestroyRemove(c, units[0])
	s.assertDoesNotNeedCleanup(c)

	// Destroy the service and check the units are unaffected, but a cleanup
	// has been scheduled.
	err := mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		err := unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
	s.assertNeedsCleanup(c)

	// Run the cleanup, and check that units are all destroyed as appropriate.
	s.assertCleanupRuns(c)
	err = units[0].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units[0].Life(), gc.Equals, state.Dying)
	err = units[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = units[2].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Run a final cleanup to clear the cleanup scheduled for the unit that
	// became dying.
	s.assertCleanupCount(c, 1)
}

func (s *CleanupSuite) TestCleanupEnvironmentServices(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a service with some units.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDoesNotNeedCleanup(c)

	// Destroy the environment and check the service and units are
	// unaffected, but a cleanup for the service has been scheduled.
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	err = mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Life(), gc.Equals, state.Dying)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(unit.Life(), gc.Equals, state.Alive)
	}

	// The first cleanup Destroys the service, which
	// schedules another cleanup to destroy the units,
	// then we need another pass for the actions cleanup
	// which is queued on the next pass
	s.assertCleanupCount(c, 2)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	// Now we should have all the cleanups done
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupRelationSettings(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a relation with a unit in scope.
	pr := NewPeerRelation(c, s.State, s.owner)
	rel := pr.ru0.Relation()
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Destroy the service, check the relation's still around.
	err = pr.svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupCount(c, 2)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// The unit leaves scope, triggering relation removal.
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Settings are not destroyed yet...
	settings, err := pr.ru1.ReadSettings("riak/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})

	// ...but they are on cleanup.
	s.assertCleanupCount(c, 1)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
}

func (s *CleanupSuite) TestForceDestroyMachineErrors(c *gc.C) {
	manager, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	// Create a relation with a unit in scope and assigned to the machine.
	pr := NewPeerRelation(c, s.State, s.owner)
	err = pr.u0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Force machine destruction, check cleanup queued.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the unit has been removed...
	s.assertCleanupCount(c, 2)
	assertRemoved(c, pr.u0)

	// ...and the unit has departed relation scope...
	assertNotJoined(c, pr.ru0)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineWithContainer(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a machine with a container.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	// Create active units (in relation scope, with subordinates).
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	err = prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the various units to machines.
	err = prr.pu0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu1.AssignToMachine(container)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Force removal of the top-level machine.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// And do it again, just to check that the second cleanup doc for the same
	// machine doesn't cause problems down the line.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the container has been removed...
	s.assertCleanupCount(c, 2)
	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

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

func (s *CleanupSuite) TestCleanupDyingUnit(c *gc.C) {
	// Create active unit, in a relation.
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy provider unit 0; check it's Dying, and a cleanup has been scheduled.
	err = prr.pu0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.pu0, state.Dying)
	s.assertNeedsCleanup(c)

	// Check it's reported in scope until cleaned up.
	assertJoined(c, prr.pru0)
	s.assertCleanupCount(c, 1)
	assertInScope(c, prr.pru0)
	assertNotJoined(c, prr.pru0)

	// Destroy the relation, and check it sticks around...
	err = prr.rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.rel, state.Dying)

	// ...until the unit is removed, and really leaves scope.
	err = prr.pu0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, prr.pru0)
	assertRemoved(c, prr.rel)
}

func (s *CleanupSuite) TestCleanupDyingUnitAlreadyRemoved(c *gc.C) {
	// Create active unit, in a relation.
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy provider unit 0; check it's Dying, and a cleanup has been scheduled.
	err = prr.pu0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.pu0, state.Dying)
	s.assertNeedsCleanup(c)

	// Remove the unit, and the relation.
	err = prr.pu0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, prr.rel)

	// Check the cleanup still runs happily.
	s.assertCleanupCount(c, 1)
}

func (s *CleanupSuite) TestCleanupActions(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a service with a unit.
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := dummy.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// Add a couple actions to the unit
	_, err = unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)

	// make sure unit still has actions
	actions, err := unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// destroy unit and run cleanups
	err = dummy.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	// make sure unit still has actions, after first cleanup pass
	actions, err = unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// second cleanup pass
	s.assertCleanupRuns(c)

	// make sure unit has no actions, after second cleanup pass
	actions, err = unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 0)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestNothingToCleanup(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) assertCleanupRuns(c *gc.C) {
	err := s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CleanupSuite) assertNeedsCleanup(c *gc.C) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsTrue)
}

func (s *CleanupSuite) assertDoesNotNeedCleanup(c *gc.C) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsFalse)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func (s *CleanupSuite) assertCleanupCount(c *gc.C, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		s.assertNeedsCleanup(c)
		s.assertCleanupRuns(c)
	}
	s.assertDoesNotNeedCleanup(c)
}
