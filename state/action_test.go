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

func (s *ActionSuite) TestActionTag(c *gc.C) {
	action, err := s.unit.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	tag := action.Tag()
	c.Assert(tag.String(), gc.Equals, "action-wordpress/0_a_0")

	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, gc.IsNil)

	r, err := s.unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(r), gc.Equals, 1)

	actionResult := r[0]
	c.Assert(actionResult, gc.DeepEquals, result)

	arTag := actionResult.Tag()
	c.Assert(arTag.String(), gc.Equals, "actionresult-wordpress/0_ar_0")
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
	c.Assert(action.Parameters(), jc.DeepEquals, params)
}

func (s *ActionSuite) TestAddActionAcceptsDuplicateNames(c *gc.C) {
	name := "fakeaction"
	params1 := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	params2 := map[string]interface{}{"infile": "infile.zip"}

	// verify can add two actions with same name
	a1, err := s.unit.AddAction(name, params1)
	c.Assert(err, gc.IsNil)

	a2, err := s.unit.AddAction(name, params2)
	c.Assert(err, gc.IsNil)

	c.Assert(a1.Id(), gc.Not(gc.Equals), a2.Id())

	// verify both actually got added
	actions, err := s.unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// verify we can Fail one, retrieve the other, and they're not mixed up
	action1, err := s.State.Action(a1.Id())
	c.Assert(err, gc.IsNil)
	_, err = action1.Finish(state.ActionResults{Status: state.ActionFailed})
	c.Assert(err, gc.IsNil)

	action2, err := s.State.Action(a2.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(action2.Parameters(), jc.DeepEquals, params2)

	// verify only one left, and it's the expected one
	actions, err = s.unit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(len(actions), gc.Equals, 1)
	c.Assert(actions[0].Id(), gc.Equals, a2.Id())
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
	result, err := action.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, gc.IsNil)

	// ensure we now have a result for this action
	results, err = unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0], gc.DeepEquals, result)

	c.Assert(results[0].Name(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	res, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, reason)
	c.Assert(res, gc.DeepEquals, map[string]interface{}{})

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
	output := map[string]interface{}{"output": "action ran successfully"}
	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted, Results: output})
	c.Assert(err, gc.IsNil)

	// ensure we now have a result for this action
	results, err = unit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0], gc.DeepEquals, result)

	c.Assert(results[0].Name(), gc.Equals, action.Name())
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	res, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res, gc.DeepEquals, output)

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

func (s *ActionSuite) TestUnitWatchActionResults(c *gc.C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit1)

	unit2, err := s.State.Unit(s.unit2.Name())
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit2)

	action0, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)
	action1, err := unit2.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)
	action2, err := unit1.AddAction("fakeaction", nil)
	c.Assert(err, gc.IsNil)

	_, err = action2.Finish(state.ActionResults{Status: state.ActionFailed})
	c.Assert(err, gc.IsNil)
	_, err = action1.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, gc.IsNil)

	w1 := unit1.WatchActionResults()
	defer statetesting.AssertStop(c, w1)
	wc1 := statetesting.NewStringsWatcherC(c, s.State, w1)
	expect := expectActionResultIds(unit1, "1")
	wc1.AssertChange(expect...)
	wc1.AssertNoChange()

	w2 := unit2.WatchActionResults()
	defer statetesting.AssertStop(c, w2)
	wc2 := statetesting.NewStringsWatcherC(c, s.State, w2)
	expect = expectActionResultIds(unit2, "0")
	wc2.AssertChange(expect...)
	wc2.AssertNoChange()

	_, err = action0.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, gc.IsNil)

	expect = expectActionResultIds(unit1, "0")
	wc1.AssertChange(expect...)
	wc1.AssertNoChange()
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
		c.Assert(err, gc.IsNil)
		c.Assert(changes, jc.DeepEquals, expected)
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
			c.Assert(err, gc.IsNil)
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

	ar1 := mockAR{id: "mock1"}
	ar2 := mockAR{id: "mock2"}
	fn = state.WatcherMakeIdFilter(s.State, marker, ar1, ar2)
	c.Assert(fn, gc.Not(gc.IsNil))

	var tests = []struct {
		id    string
		match bool
	}{
		{id: "mock1" + marker + "", match: true},
		{id: "mock1" + marker + "asdf", match: true},
		{id: "mock2" + marker + "", match: true},
		{id: "mock2" + marker + "asdf", match: true},

		{id: "mock1" + badmarker + "", match: false},
		{id: "mock1" + badmarker + "asdf", match: false},
		{id: "mock2" + badmarker + "", match: false},
		{id: "mock2" + badmarker + "asdf", match: false},

		{id: "mock1" + marker + "0", match: true},
		{id: "mock10" + marker + "0", match: false},
		{id: "mock2" + marker + "0", match: true},
		{id: "mock20" + marker + "0", match: false},
		{id: "mock" + marker + "0", match: false},

		{id: "" + marker + "0", match: false},
		{id: "mock1-0", match: false},
		{id: "mock1-0", match: false},
	}

	for _, test := range tests {
		c.Assert(fn(state.DocID(s.State, test.id)), gc.Equals, test.match)
	}
}

func (s *ActionSuite) TestWatchActions(c *gc.C) {
	svc := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	w := s.State.WatchActions()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// add 3 actions
	_, err = u.AddAction("fakeaction1", nil)
	c.Assert(err, gc.IsNil)
	fa2, err := u.AddAction("fakeaction2", nil)
	c.Assert(err, gc.IsNil)
	_, err = u.AddAction("fakeaction3", nil)
	c.Assert(err, gc.IsNil)

	// fail the middle one
	action, err := s.State.Action(fa2.Id())
	c.Assert(err, gc.IsNil)
	_, err = action.Finish(state.ActionResults{Status: state.ActionFailed, Message: "die scum"})
	c.Assert(err, gc.IsNil)

	// expect the first and last one in the watcher
	expect := expectActionIds(u, "0", "2")
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}

func expectActionIds(u *state.Unit, suffixes ...string) []string {
	ids := make([]string, len(suffixes))
	prefix := state.EnsureActionMarker(u.Name())
	for i, suffix := range suffixes {
		ids[i] = prefix + suffix
	}
	return ids
}

func expectActionResultIds(u *state.Unit, suffixes ...string) []string {
	ids := make([]string, len(suffixes))
	prefix := state.EnsureActionResultMarker(u.Name())
	for i, suffix := range suffixes {
		ids[i] = prefix + suffix
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
// testing that requires the ActionReceiver.Name() call to return an id
type mockAR struct {
	id string
}

var _ state.ActionReceiver = (*mockAR)(nil)

func (r mockAR) AddAction(name string, payload map[string]interface{}) (*state.Action, error) {
	return nil, nil
}

func (r mockAR) CancelAction(*state.Action) (*state.ActionResult, error) { return nil, nil }
func (r mockAR) WatchActions() state.StringsWatcher                      { return nil }
func (r mockAR) WatchActionResults() state.StringsWatcher                { return nil }
func (r mockAR) Actions() ([]*state.Action, error)                       { return nil, nil }
func (r mockAR) ActionResults() ([]*state.ActionResult, error)           { return nil, nil }
func (r mockAR) Name() string                                            { return r.id }
func (r mockAR) Tag() names.Tag                                          { return nil }
