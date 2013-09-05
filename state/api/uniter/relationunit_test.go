// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	statetesting "launchpad.net/juju-core/state/testing"
)

type relationUnitSuite struct {
	uniterSuite

	mysqlMachine *state.Machine
	mysqlService *state.Service
	mysqlCharm   *state.Charm
	mysqlUnit    *state.Unit
}

var _ = gc.Suite(&relationUnitSuite{})

func (s *relationUnitSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	// Now create another machine, service and unit, so we can
	// test relations and relation units.
	s.mysqlMachine, s.mysqlService, s.mysqlCharm, s.mysqlUnit = s.addMachineServiceCharmAndUnit(c, "mysql")
}

func (s *relationUnitSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *relationUnitSuite) TestWatchRelationUnits(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	myRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	apiRel, err := s.uniter.Relation(rel.Tag())
	c.Assert(err, gc.IsNil)
	apiUnit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)

	w, err := apiRelUnit.Watch()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewRelationUnitsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange([]string{"mysql/0"}, nil)

	// Leave scope with mysqlUnit, check it's detected.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, false)
	wc.AssertChange(nil, []string{"mysql/0"})

	// Non-change is not reported.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: This test is not as exhaustive as the one in state,
	// because the watcher is already tested there. Here we just
	// ensure we get the events when we expect them and don't get
	// them when they're not expected.

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
