// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/txn"
)

type ActionSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
	unit2   *state.Unit
}

var _ = gc.Suite(&ActionSuite{})

func (s *ActionSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	c.Assert(err, gc.IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
	s.unit2, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit2.Series(), gc.Equals, "quantal")
}

func (s *ActionSuite) TestAddAction(c *gc.C) {
	name := "fakeaction"
	params := map[string]interface{}{"outfile": "outfile.tar.bz2"}

	// verify can add an Action
	a, err := s.unit.AddAction(name, params)
	c.Assert(err, gc.IsNil)

	// verify we can get it back out by Id
	action, err := s.State.Action(a.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(action, gc.NotNil)
	c.Assert(action.Id(), gc.Equals, a.Id())

	// verify we get out what we put in
	c.Assert(action.Name(), gc.Equals, name)
	c.Assert(action.Payload(), jc.DeepEquals, params)
}

func (s *ActionSuite) TestAddActionAcceptsDuplicateNames(c *gc.C) {
	name := "fakeaction"
	params_1 := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	params_2 := map[string]interface{}{"infile": "infile.zip"}

	// verify can add two actions with same name
	a_1, err := s.unit.AddAction(name, params_1)
	c.Assert(err, gc.IsNil)

	a_2, err := s.unit.AddAction(name, params_2)
	c.Assert(err, gc.IsNil)

	c.Assert(a_1.Id(), gc.Not(gc.Equals), a_2.Id())

	// verify both actually got added
	actions, err := s.unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// verify we can Fail one, retrieve the other, and they're not mixed up
	action_1, err := s.State.Action(a_1.Id())
	c.Assert(err, gc.IsNil)
	err = action_1.Fail("")
	c.Assert(err, gc.IsNil)

	action_2, err := s.State.Action(a_2.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(action_2.Payload(), gc.DeepEquals, params_2)

	// verify only one left, and it's the expected one
	actions, err = s.unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 1)
	c.Assert(actions[0].Id(), gc.Equals, a_2.Id())
}

func (s *ActionSuite) TestAddActionLifecycle(c *gc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	// make unit state Dying
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)

	// can add action to a dying unit
	_, err = unit.AddAction("fakeaction1", map[string]interface{}{})
	c.Assert(err, gc.IsNil)

	// make sure unit is dead
	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)

	// cannot add action to a dead unit
	_, err = unit.AddAction("fakeaction2", map[string]interface{}{})
	c.Assert(err, gc.ErrorMatches, "unit .* is dead")
}

func (s *ActionSuite) TestAddActionFailsOnDeadUnitInTransaction(c *gc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	killUnit := txn.TestHook{
		Before: func() {
			c.Assert(unit.Destroy(), gc.IsNil)
			c.Assert(unit.EnsureDead(), gc.IsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, killUnit).Check()

	_, err = unit.AddAction("fakeaction", map[string]interface{}{})
	c.Assert(err, gc.ErrorMatches, "unit .* is dead")
}

func (s *ActionSuite) TestFail(c *gc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	a, err := unit.AddAction("action1", nil)
	c.Assert(err, gc.IsNil)

	action, err := s.State.Action(a.Id())
	c.Assert(err, gc.IsNil)

	// ensure no action results for this action
	results, err := unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 0)

	// fail the action, and verify that it succeeds
	reason := "test fail reason"
	err = action.Fail(reason)
	c.Assert(err, gc.IsNil)

	// ensure we now have a result for this action
	results, err = unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)

	c.Assert(results[0].ActionName(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	c.Assert(results[0].Output(), gc.Equals, reason)

	// validate that a failed action is no longer returned by UnitActions.
	actions, err := unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 0)
}

func (s *ActionSuite) TestComplete(c *gc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	a, err := unit.AddAction("action1", nil)
	c.Assert(err, gc.IsNil)

	action, err := s.State.Action(a.Id())
	c.Assert(err, gc.IsNil)

	// ensure no action results for this action
	results, err := unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 0)

	// complete the action, and verify that it succeeds
	output := "action ran successfully"
	err = action.Complete(output)
	c.Assert(err, gc.IsNil)

	// ensure we now have a result for this action
	results, err = unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)

	c.Assert(results[0].ActionName(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	c.Assert(results[0].Output(), gc.Equals, output)

	// validate that a completed action is no longer returned by UnitActions.
	actions, err := unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 0)
}

func (s *ActionSuite) TestUnitWatchActions(c *gc.C) {
	// get units
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit1)

	unit2, err := s.State.Unit(s.unit2.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit2)

	// queue some actions before starting the watcher
	_, err = unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)
	_, err = unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	// set up watcher on first unit
	w := unit1.WatchActions()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	// make sure the previously pending actions are sent on the watcher
	expect := expectActionIds(unit1, "0", "1")
	wc.AssertChange(expect...)
	wc.AssertNoChange()

	// add watcher on unit2
	w2 := unit2.WatchActions()
	defer statetesting.AssertStop(c, w2)
	wc2 := statetesting.NewStringsWatcherC(c, s.State, w2)
	wc2.AssertChange()
	wc2.AssertNoChange()

	// add action on unit2 and makes sure unit1 watcher doesn't trigger
	// and unit2 watcher does
	_, err = unit2.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
	expect2 := expectActionIds(unit2, "0")
	wc2.AssertChange(expect2...)
	wc2.AssertNoChange()

	// add a couple actions on unit1 and make sure watcher sees events
	_, err = unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)
	_, err = unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	expect = expectActionIds(unit1, "2", "3")
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}
