// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
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
	c.Assert(err, jc.ErrorIsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
	s.unit2, err = s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit2.Series(), gc.Equals, "quantal")
}

func (s *ActionSuite) TestActionTag(c *gc.C) {
	action, err := s.unit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	tag := action.Tag()
	c.Assert(tag.String(), gc.Equals, "action-"+action.Id())

	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, jc.ErrorIsNil)

	actions, err := s.unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 1)

	actionResult := actions[0]
	c.Assert(actionResult, gc.DeepEquals, result)

	tag = actionResult.Tag()
	c.Assert(tag.String(), gc.Equals, "action-"+actionResult.Id())
}

func (s *ActionSuite) TestAddAction(c *gc.C) {
	name := "fakeaction"
	params := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	before := state.NowToTheSecond()
	later := before.Add(testing.LongWait)

	// verify can add an Action
	a, err := s.unit.AddAction(name, params)
	c.Assert(err, jc.ErrorIsNil)

	// verify we can get it back out by Id
	action, err := s.State.Action(a.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action, gc.NotNil)
	c.Assert(action.Id(), gc.Equals, a.Id())

	// verify we get out what we put in
	c.Assert(action.Name(), gc.Equals, name)
	c.Assert(action.Parameters(), jc.DeepEquals, params)

	// Enqueued time should be within a reasonable time of the beginning
	// of the test
	now := state.NowToTheSecond()
	c.Check(action.Enqueued(), jc.TimeBetween(before, now))
	c.Check(action.Enqueued(), jc.TimeBetween(before, later))
}

func (s *ActionSuite) TestAddActionAcceptsDuplicateNames(c *gc.C) {
	name := "fakeaction"
	params1 := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	params2 := map[string]interface{}{"infile": "infile.zip"}

	// verify can add two actions with same name
	a1, err := s.unit.AddAction(name, params1)
	c.Assert(err, jc.ErrorIsNil)

	a2, err := s.unit.AddAction(name, params2)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(a1.Id(), gc.Not(gc.Equals), a2.Id())

	// verify both actually got added
	actions, err := s.unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// verify we can Fail one, retrieve the other, and they're not mixed up
	action1, err := s.State.Action(a1.Id())
	c.Assert(err, jc.ErrorIsNil)
	_, err = action1.Finish(state.ActionResults{Status: state.ActionFailed})
	c.Assert(err, jc.ErrorIsNil)

	action2, err := s.State.Action(a2.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action2.Parameters(), jc.DeepEquals, params2)

	// verify only one left, and it's the expected one
	actions, err = s.unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 1)
	c.Assert(actions[0].Id(), gc.Equals, a2.Id())
}

func (s *ActionSuite) TestAddActionLifecycle(c *gc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	// make unit state Dying
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// can add action to a dying unit
	_, err = unit.AddAction("fakeaction1", map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	// make sure unit is dead
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// cannot add action to a dead unit
	_, err = unit.AddAction("fakeaction2", map[string]interface{}{})
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *ActionSuite) TestAddActionFailsOnDeadUnitInTransaction(c *gc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	killUnit := txn.TestHook{
		Before: func() {
			c.Assert(unit.Destroy(), gc.IsNil)
			c.Assert(unit.EnsureDead(), gc.IsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, killUnit).Check()

	_, err = unit.AddAction("fakeaction", map[string]interface{}{})
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *ActionSuite) TestFail(c *gc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	a, err := unit.AddAction("action1", nil)
	c.Assert(err, jc.ErrorIsNil)

	action, err := s.State.Action(a.Id())
	c.Assert(err, jc.ErrorIsNil)

	// ensure no action results for this action
	results, err := unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 0)

	// fail the action, and verify that it succeeds
	reason := "test fail reason"
	result, err := action.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, jc.ErrorIsNil)

	// ensure we now have a result for this action
	results, err = unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0], gc.DeepEquals, result)

	c.Assert(results[0].Name(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)

	// Verify the Action Completed time was within a reasonable
	// time of the Enqueued time.
	diff := results[0].Completed().Sub(action.Enqueued())
	c.Assert(diff >= 0, jc.IsTrue)
	c.Assert(diff < testing.LongWait, jc.IsTrue)

	res, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, reason)
	c.Assert(res, gc.DeepEquals, map[string]interface{}{})

	// validate that a pending action is no longer returned by UnitActions.
	actions, err := unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 0)
}

func (s *ActionSuite) TestComplete(c *gc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	a, err := unit.AddAction("action1", nil)
	c.Assert(err, jc.ErrorIsNil)

	action, err := s.State.Action(a.Id())
	c.Assert(err, jc.ErrorIsNil)

	// ensure no action results for this action
	results, err := unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 0)

	// complete the action, and verify that it succeeds
	output := map[string]interface{}{"output": "action ran successfully"}
	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted, Results: output})
	c.Assert(err, jc.ErrorIsNil)

	// ensure we now have a result for this action
	results, err = unit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0], gc.DeepEquals, result)

	c.Assert(results[0].Name(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	res, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res, gc.DeepEquals, output)

	// validate that a pending action is no longer returned by UnitActions.
	actions, err := unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 0)
}

func (s *ActionSuite) TestActionsWatcherEmitsInitialChanges(c *gc.C) {
	// LP-1391914 :: idPrefixWatcher fails watcher contract to send
	// initial Change event
	//
	// state/idPrefixWatcher does not send an initial event in response
	// to the first time Changes() is called if all of the pending
	// events are removed before the first consumption of Changes().
	// The watcher contract specifies that the first call to Changes()
	// should always return at a minimum an empty change set to notify
	// clients of it's initial state

	// preamble
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit1)

	// queue up actions
	a1, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	a2, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	// start watcher but don't consume Changes() yet
	w := unit1.WatchActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	// remove actions
	reason := "removed"
	_, err = a1.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, jc.ErrorIsNil)
	_, err = a2.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, jc.ErrorIsNil)

	// per contract, there should be at minimum an initial empty Change() result
	wc.AssertChange()
	wc.AssertNoChange()
}

func (s *ActionSuite) TestUnitWatchActionNotifications(c *gc.C) {
	// get units
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit1)

	unit2, err := s.State.Unit(s.unit2.Name())
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit2)

	// queue some actions before starting the watcher
	fa1, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	fa2, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	// set up watcher on first unit
	w := unit1.WatchActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	// make sure the previously pending actions are sent on the watcher
	expect := expectActionIds(fa1, fa2)
	wc.AssertChange(expect...)
	wc.AssertNoChange()

	// add watcher on unit2
	w2 := unit2.WatchActionNotifications()
	defer statetesting.AssertStop(c, w2)
	wc2 := statetesting.NewStringsWatcherC(c, s.State, w2)
	wc2.AssertChange()
	wc2.AssertNoChange()

	// add action on unit2 and makes sure unit1 watcher doesn't trigger
	// and unit2 watcher does
	fa3, err := unit2.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	expect2 := expectActionIds(fa3)
	wc2.AssertChange(expect2...)
	wc2.AssertNoChange()

	// add a couple actions on unit1 and make sure watcher sees events
	fa4, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	fa5, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	expect = expectActionIds(fa4, fa5)
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}

func (s *ActionSuite) TestMergeIds(c *gc.C) {
	var tests = []struct {
		changes  string
		adds     string
		removes  string
		expected string
	}{
		{changes: "", adds: "a0,a1", removes: "", expected: "a0,a1"},
		{changes: "a0,a1", adds: "", removes: "a0", expected: "a1"},
		{changes: "a0,a1", adds: "a2", removes: "a0", expected: "a1,a2"},

		{changes: "", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "", adds: "a0,a1,a2", removes: "a0,a1,a2", expected: ""},

		{changes: "a0", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "a1", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "a2", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},

		{changes: "a3,a4", adds: "a1,a4,a5", removes: "a1,a3", expected: "a4,a5"},
		{changes: "a0,a1,a2", adds: "a1,a4,a5", removes: "a1,a3", expected: "a0,a2,a4,a5"},
	}

	for ix, test := range tests {
		updates := mapify(test.adds, test.removes)
		changes := sliceify(test.changes)
		expected := sliceify(test.expected)

		c.Log(fmt.Sprintf("test number %d %#v", ix, test))
		err := state.WatcherMergeIds(s.State, &changes, updates)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(changes, jc.SameContents, expected)
	}
}

func (s *ActionSuite) TestMergeIdsErrors(c *gc.C) {

	var tests = []struct {
		ok   bool
		name string
		key  interface{}
	}{
		{ok: false, name: "bool", key: true},
		{ok: false, name: "int", key: 0},
		{ok: false, name: "chan string", key: make(chan string)},

		{ok: true, name: "string", key: ""},
	}

	for _, test := range tests {
		changes, updates := []string{}, map[interface{}]bool{}

		updates[test.key] = true
		err := state.WatcherMergeIds(s.State, &changes, updates)

		if test.ok {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, "id is not of type string, got "+test.name)
		}
	}
}

func (s *ActionSuite) TestEnsureSuffix(c *gc.C) {
	marker := "-marker-"
	fn := state.WatcherEnsureSuffixFn(marker)
	c.Assert(fn, gc.Not(gc.IsNil))

	var tests = []struct {
		given  string
		expect string
	}{
		{given: marker, expect: marker},
		{given: "", expect: "" + marker},
		{given: "asdf", expect: "asdf" + marker},
		{given: "asdf" + marker, expect: "asdf" + marker},
		{given: "asdf" + marker + "qwerty", expect: "asdf" + marker + "qwerty" + marker},
	}

	for _, test := range tests {
		c.Assert(fn(test.given), gc.Equals, test.expect)
	}
}

func (s *ActionSuite) TestMakeIdFilter(c *gc.C) {
	marker := "-marker-"
	badmarker := "-bad-"
	fn := state.WatcherMakeIdFilter(s.State, marker)
	c.Assert(fn, gc.IsNil)

	ar1 := mockAR{id: "mock/1"}
	ar2 := mockAR{id: "mock/2"}
	fn = state.WatcherMakeIdFilter(s.State, marker, ar1, ar2)
	c.Assert(fn, gc.Not(gc.IsNil))

	var tests = []struct {
		id    string
		match bool
	}{
		{id: "mock/1" + marker + "", match: true},
		{id: "mock/1" + marker + "asdf", match: true},
		{id: "mock/2" + marker + "", match: true},
		{id: "mock/2" + marker + "asdf", match: true},

		{id: "mock/1" + badmarker + "", match: false},
		{id: "mock/1" + badmarker + "asdf", match: false},
		{id: "mock/2" + badmarker + "", match: false},
		{id: "mock/2" + badmarker + "asdf", match: false},

		{id: "mock/1" + marker + "0", match: true},
		{id: "mock/10" + marker + "0", match: false},
		{id: "mock/2" + marker + "0", match: true},
		{id: "mock/20" + marker + "0", match: false},
		{id: "mock" + marker + "0", match: false},

		{id: "" + marker + "0", match: false},
		{id: "mock/1-0", match: false},
		{id: "mock/1-0", match: false},
	}

	for _, test := range tests {
		c.Assert(fn(state.DocID(s.State, test.id)), gc.Equals, test.match)
	}
}

func (s *ActionSuite) TestWatchActionNotifications(c *gc.C) {
	svc := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	w := u.WatchActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// add 3 actions
	fa1, err := u.AddAction("fakeaction1", nil)
	c.Assert(err, jc.ErrorIsNil)
	fa2, err := u.AddAction("fakeaction2", nil)
	c.Assert(err, jc.ErrorIsNil)
	fa3, err := u.AddAction("fakeaction3", nil)
	c.Assert(err, jc.ErrorIsNil)

	// fail the middle one
	action, err := s.State.Action(fa2.Id())
	c.Assert(err, jc.ErrorIsNil)
	_, err = action.Finish(state.ActionResults{Status: state.ActionFailed, Message: "die scum"})
	c.Assert(err, jc.ErrorIsNil)

	// expect the first and last one in the watcher
	expect := expectActionIds(fa1, fa3)
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}

func (s *ActionSuite) TestActionStatusWatcher(c *gc.C) {
	testCase := []struct {
		receiver state.ActionReceiver
		name     string
		status   state.ActionStatus
	}{
		{s.unit, "fake-action-1", state.ActionCancelled},
		{s.unit2, "poseur-3", state.ActionCancelled},
		{s.unit, "fake-action-2", state.ActionPending},
		{s.unit2, "other-3", state.ActionPending},
		{s.unit, "fake-action-3", state.ActionFailed},
		{s.unit2, "stooge-3", state.ActionFailed},
		{s.unit, "fake-action-4", state.ActionCompleted},
		{s.unit2, "bunny-3", state.ActionCompleted},
	}

	w1 := state.NewActionStatusWatcher(s.State, []state.ActionReceiver{s.unit})
	defer statetesting.AssertStop(c, w1)

	w2 := state.NewActionStatusWatcher(s.State, []state.ActionReceiver{s.unit}, state.ActionFailed)
	defer statetesting.AssertStop(c, w2)

	w3 := state.NewActionStatusWatcher(s.State, []state.ActionReceiver{s.unit}, state.ActionCancelled, state.ActionCompleted)
	defer statetesting.AssertStop(c, w3)

	watchAny := statetesting.NewStringsWatcherC(c, s.State, w1)
	watchAny.AssertChange()
	watchAny.AssertNoChange()

	watchFailed := statetesting.NewStringsWatcherC(c, s.State, w2)
	watchFailed.AssertChange()
	watchFailed.AssertNoChange()

	watchCancelledOrCompleted := statetesting.NewStringsWatcherC(c, s.State, w3)
	watchCancelledOrCompleted.AssertChange()
	watchCancelledOrCompleted.AssertNoChange()

	expect := map[state.ActionStatus][]*state.Action{}
	all := []*state.Action{}
	for _, tcase := range testCase {
		a, err := tcase.receiver.AddAction(tcase.name, nil)
		c.Assert(err, jc.ErrorIsNil)

		action, err := s.State.Action(a.Id())
		c.Assert(err, jc.ErrorIsNil)

		_, err = action.Finish(state.ActionResults{Status: tcase.status})
		c.Assert(err, jc.ErrorIsNil)

		if tcase.receiver == s.unit {
			expect[tcase.status] = append(expect[tcase.status], action)
			all = append(all, action)
		}
	}

	watchAny.AssertChange(expectActionIds(all...)...)
	watchAny.AssertNoChange()

	watchFailed.AssertChange(expectActionIds(expect[state.ActionFailed]...)...)
	watchFailed.AssertNoChange()

	cancelledAndCompleted := expectActionIds(append(expect[state.ActionCancelled], expect[state.ActionCompleted]...)...)
	watchCancelledOrCompleted.AssertChange(cancelledAndCompleted...)
	watchCancelledOrCompleted.AssertNoChange()
}

func expectActionIds(actions ...*state.Action) []string {
	ids := make([]string, len(actions))
	for i, action := range actions {
		ids[i] = action.Id()
	}
	return ids
}

// mapify is a convenience method, also to make reading the tests
// easier. It combines two comma delimited strings representing
// additions and removals and turns it into the map[interface{}]bool
// format needed
func mapify(adds, removes string) map[interface{}]bool {
	m := map[interface{}]bool{}
	for _, v := range sliceify(adds) {
		m[v] = true
	}
	for _, v := range sliceify(removes) {
		m[v] = false
	}
	return m
}

// sliceify turns a comma separated list of strings into a slice
// trimming white space and excluding empty strings.
func sliceify(csvlist string) []string {
	slice := []string{}
	if csvlist == "" {
		return slice
	}
	for _, entry := range strings.Split(csvlist, ",") {
		clean := strings.TrimSpace(entry)
		if clean != "" {
			slice = append(slice, clean)
		}
	}
	return slice
}

// mockAR is an implementation of ActionReceiver that can be used for
// testing that requires the ActionReceiver.Tag() call to return a
// names.Tag
type mockAR struct {
	id string
}

var _ state.ActionReceiver = (*mockAR)(nil)

func (r mockAR) AddAction(name string, payload map[string]interface{}) (*state.Action, error) {
	return nil, nil
}
func (r mockAR) CancelAction(*state.Action) (*state.Action, error) { return nil, nil }
func (r mockAR) WatchActionNotifications() state.StringsWatcher    { return nil }
func (r mockAR) Actions() ([]*state.Action, error)                 { return nil, nil }
func (r mockAR) CompletedActions() ([]*state.Action, error)        { return nil, nil }
func (r mockAR) PendingActions() ([]*state.Action, error)          { return nil, nil }
func (r mockAR) Tag() names.Tag                                    { return names.NewUnitTag(r.id) }
