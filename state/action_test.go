// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type ActionSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
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
}

func (s *ActionSuite) TestAddAction(c *gc.C) {
	name := "fakeaction"
	params := map[string]interface{}{"outfile": "outfile.tar.bz2"}

	// verify can add an Action
	id, err := s.unit.AddAction(name, params)
	c.Assert(err, gc.IsNil)
	assertSaneActionId(c, id, s.unit.Name())

	// verify we can get it back out by Id
	action, err := s.State.Action(id)
	c.Assert(err, gc.IsNil)
	c.Assert(action, gc.NotNil)
	c.Assert(action.Id(), gc.Equals, id)

	// verify we get out what we put in
	c.Assert(action.Name(), gc.Equals, name)
	c.Assert(action.Payload(), jc.DeepEquals, params)
}

func (s *ActionSuite) TestAddActionAcceptsDuplicateNames(c *gc.C) {
	name := "fakeaction"
	params_1 := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	params_2 := map[string]interface{}{"infile": "infile.zip"}

	// verify can add two actions with same name
	id_1, err := s.unit.AddAction(name, params_1)
	c.Assert(err, gc.IsNil)
	assertSaneActionId(c, id_1, s.unit.Name())

	id_2, err := s.unit.AddAction(name, params_2)
	c.Assert(err, gc.IsNil)
	assertSaneActionId(c, id_2, s.unit.Name())

	c.Assert(id_1, gc.Not(gc.Equals), id_2)

	// verify both actually got added
	actions, err := s.State.UnitActions(s.unit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// verify we can Fail one, retrieve the other, and they're not mixed up
	action_1, err := s.State.Action(id_1)
	c.Assert(err, gc.IsNil)
	err = action_1.Fail("")
	c.Assert(err, gc.IsNil)

	action_2, err := s.State.Action(id_2)
	c.Assert(err, gc.IsNil)
	c.Assert(action_2.Payload(), gc.DeepEquals, params_2)

	// verify only one left, and it's the expected one
	actions, err = s.State.UnitActions(s.unit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 1)
	c.Assert(actions[0].Id(), gc.Equals, id_2)
}

func (s *ActionSuite) TestAddActionLifecycle(c *gc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	// make unit state Dying
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)

	// can add action to a dying unit
	id, err := unit.AddAction("fakeaction1", map[string]interface{}{})
	c.Assert(err, gc.IsNil)
	assertSaneActionId(c, id, s.unit.Name())

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

	killUnit := state.TransactionHook{
		Before: func() {
			c.Assert(unit.Destroy(), gc.IsNil)
			c.Assert(unit.EnsureDead(), gc.IsNil)
		},
	}
	defer state.SetTransactionHooks(c, s.State, killUnit).Check()

	_, err = unit.AddAction("fakeaction", map[string]interface{}{})
	c.Assert(err, gc.ErrorMatches, "unit .* is dead")
}

func (s *ActionSuite) TestFail(c *gc.C) {
	// TODO(jcw4): when action results are implemented we should be
	// checking for a Fail result after calling Fail(), rather than
	// sniffing the logs
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("actions-tester", tw, loggo.DEBUG), gc.IsNil)

	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)

	id, err := unit.AddAction("action1", nil)
	c.Assert(err, gc.IsNil)

	action, err := s.State.Action(id)
	c.Assert(err, gc.IsNil)

	// fail the action, and verify that it succeeds (right now, just by
	// sniffing the logs)
	reason := "test fail reason"
	err = action.Fail(reason)
	c.Assert(err, gc.IsNil)
	// TODO(jcw4): replace with action results check when they're implemented
	c.Assert(tw.Log, jc.LogMatches, jc.SimpleMessages{{loggo.WARNING, reason}})

	// validate that a failed action is no longer returned by UnitActions.
	actions, err := s.State.UnitActions(unit.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 0)
}

// assertSaneActionId verifies that the id is of the expected
// form (unit id prefix + sequence)
// This is a temporary assertion, we shouldn't be leaking the actual
// mongo _id
func assertSaneActionId(c *gc.C, id, unitName string) {
	c.Assert(id, gc.Matches, "^u#"+unitName+"#a#\\d+")
}
