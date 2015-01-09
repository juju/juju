// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type actionSuite struct {
	uniterSuite
}

var _ = gc.Suite(&actionSuite{})
var basicParams = map[string]interface{}{"outfile": "foo.txt"}

func (s *actionSuite) TestAction(c *gc.C) {
	var actionTests = []struct {
		description string
		action      params.Action
	}{{
		description: "A simple Action.",
		action: params.Action{
			Name:       "fakeaction",
			Parameters: basicParams,
		},
	}, {
		description: "An Action with nested parameters.",
		action: params.Action{
			Name: "fakeaction",
			Parameters: map[string]interface{}{
				"outfile": "foo.bz2",
				"compression": map[string]interface{}{
					"kind":    "bzip",
					"quality": float64(5.0),
				},
			},
		},
	}}

	for i, actionTest := range actionTests {
		c.Logf("test %d: %s", i, actionTest.description)
		a, err := s.uniterSuite.wordpressUnit.AddAction(
			actionTest.action.Name,
			actionTest.action.Parameters)
		c.Assert(err, jc.ErrorIsNil)

		ok := names.IsValidAction(a.Id())
		c.Assert(ok, gc.Equals, true)
		actionTag := names.NewActionTag(a.Id())
		c.Assert(a.Tag(), gc.Equals, actionTag)

		retrievedAction, err := s.uniter.Action(actionTag)
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(retrievedAction.Name(), gc.DeepEquals, actionTest.action.Name)
		c.Assert(retrievedAction.Params(), gc.DeepEquals, actionTest.action.Parameters)
	}
}

func (s *actionSuite) TestActionNotFound(c *gc.C) {
	_, err := s.uniter.Action(names.NewActionTag("feedface-0123-4567-8901-2345deadbeef"))
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, `action "feedface-0123-4567-8901-2345deadbeef" not found`)
}

func (s *actionSuite) TestNewActionAndAccessors(c *gc.C) {
	testAction, err := uniter.NewAction("snapshot", basicParams)
	c.Assert(err, jc.ErrorIsNil)
	testName := testAction.Name()
	testParams := testAction.Params()
	c.Assert(testName, gc.Equals, "snapshot")
	c.Assert(testParams, gc.DeepEquals, basicParams)
}

func (s *actionSuite) TestActionComplete(c *gc.C) {
	completed, err := s.uniterSuite.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(completed, gc.DeepEquals, ([]*state.Action)(nil))

	action, err := s.uniterSuite.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	actionResult := map[string]interface{}{"output": "it worked!"}
	err = s.uniter.ActionFinish(action.ActionTag(), params.ActionCompleted, actionResult, "")
	c.Assert(err, jc.ErrorIsNil)

	completed, err = s.uniterSuite.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(completed), gc.Equals, 1)
	c.Assert(completed[0].Status(), gc.Equals, state.ActionCompleted)
	res, errstr := completed[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res, gc.DeepEquals, actionResult)
	c.Assert(completed[0].Name(), gc.Equals, "fakeaction")
}

func (s *actionSuite) TestActionFail(c *gc.C) {
	completed, err := s.uniterSuite.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(completed, gc.DeepEquals, ([]*state.Action)(nil))

	action, err := s.uniterSuite.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	errmsg := "it failed!"
	err = s.uniter.ActionFinish(action.ActionTag(), params.ActionFailed, nil, errmsg)
	c.Assert(err, jc.ErrorIsNil)

	completed, err = s.uniterSuite.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(completed), gc.Equals, 1)
	c.Assert(completed[0].Status(), gc.Equals, state.ActionFailed)
	res, errstr := completed[0].Results()
	c.Assert(errstr, gc.Equals, errmsg)
	c.Assert(res, gc.DeepEquals, map[string]interface{}{})
	c.Assert(completed[0].Name(), gc.Equals, "fakeaction")
}
