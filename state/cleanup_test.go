package state_test

import (
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
	s.assertNeedsCleanup(c, false)

	// Create a service with some units.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit()
		c.Assert(err, gc.IsNil)
		units[i] = unit
	}
	preventUnitDestroyRemove(c, units[0])
	s.assertNeedsCleanup(c, false)

	// Destroy the service and check the units are unaffected, but a cleanup
	// has been scheduled.
	err = mysql.Destroy()
	c.Assert(err, gc.IsNil)
	for _, unit := range units {
		err := unit.Refresh()
		c.Assert(err, gc.IsNil)
	}
	s.assertNeedsCleanup(c, true)

	// Run the cleanup, and check that units are all destroyed as appropriate.
	s.assertCleanup(c)
	s.assertNeedsCleanup(c, false)
	err = units[0].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(units[0].Life(), gc.Equals, state.Dying)
	err = units[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = units[2].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *CleanupSuite) TestCleanupRelationSettings(c *gc.C) {
	s.assertNeedsCleanup(c, false)

	// Create a relation with a unit in scope.
	pr := NewPeerRelation(c, s.State)
	rel := pr.ru0.Relation()
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c, false)

	// Destroy the service, check the relation's still around.
	err = pr.svc.Destroy()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c, true)
	s.assertCleanup(c)
	err = rel.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)
	s.assertNeedsCleanup(c, false)

	// The unit leaves scope, triggering relation removal.
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c, true)

	// Settings are not destroyed yet...
	settings, err := pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})

	// ...but they are on cleanup.
	s.assertCleanup(c)
	s.assertNeedsCleanup(c, false)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachine(c *gc.C) {
	s.assertNeedsCleanup(c, false)

	// Create a machine with a container.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	container, err := s.State.AddMachineWithConstraints(&state.AddMachineParams{
		Series:        "quantal",
		ParentId:      machine.Id(),
		ContainerType: instance.LXC,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	})
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
	s.assertNeedsCleanup(c, false)

	// Force removal of the top-level machine.
	err = s.State.ForceDestroyMachines(machine.Id())
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c, true)

	// And do it again, just to check that the second cleanup doc for the same
	// machine doesn't cause problems down the line.
	err = s.State.ForceDestroyMachines(machine.Id())
	c.Assert(err, gc.IsNil)
	s.assertNeedsCleanup(c, true)

	// Clean up, and check that all the machines have been removed...
	s.assertCleanup(c)
	s.assertNeedsCleanup(c, false)
	err = machine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	// ...and so have all the units...
	assertUnitRemoved(c, prr.pu0)
	assertUnitRemoved(c, prr.pu1)
	assertUnitRemoved(c, prr.ru0)
	assertUnitRemoved(c, prr.ru1)

	// ...and none of the units have left relation scopes occupied.
	assertInScope(c, prr.pru0, false)
	assertInScope(c, prr.pru1, false)
	assertInScope(c, prr.rru0, false)
	assertInScope(c, prr.rru1, false)
}

func (s *CleanupSuite) TestNothingToCleanup(c *gc.C) {
	s.assertNeedsCleanup(c, false)
	s.assertCleanup(c)
	s.assertNeedsCleanup(c, false)
}

func (s *CleanupSuite) assertCleanup(c *gc.C) {
	err := s.State.Cleanup()
	c.Assert(err, gc.IsNil)
}

func (s *CleanupSuite) assertNeedsCleanup(c *gc.C, expect bool) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.Equals, expect)
}
