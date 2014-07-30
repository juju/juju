// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
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
			Name:   "snapshot",
			Params: basicParams,
		},
	}, {
		description: "An Action with nested parameters.",
		action: params.Action{
			Name: "backup",
			Params: map[string]interface{}{
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
			actionTest.action.Params)
		c.Assert(err, gc.IsNil)

		actionTag := names.JoinActionTag(s.uniterSuite.wordpressUnit.Name(), i)
		c.Assert(a.Tag(), gc.Equals, actionTag)

		retrievedAction, err := s.uniter.Action(actionTag)
		c.Assert(err, gc.IsNil)

		c.Assert(retrievedAction.Name(), gc.DeepEquals, actionTest.action.Name)
		c.Assert(retrievedAction.Params(), gc.DeepEquals, actionTest.action.Params)
	}
}

func (s *actionSuite) TestActionNotFound(c *gc.C) {
	_, err := s.uniter.Action(names.JoinActionTag("wordpress/0", 0))
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "action .*wordpress/0[^0-9]+0[^0-9]+ not found")
}

func (s *actionSuite) TestNewActionAndAccessors(c *gc.C) {
	testAction, err := uniter.NewAction("snapshot", basicParams)
	c.Assert(err, gc.IsNil)
	testName := testAction.Name()
	testParams := testAction.Params()
	c.Assert(testName, gc.Equals, "snapshot")
	c.Assert(testParams, gc.DeepEquals, basicParams)
}

func (s *actionSuite) TestActionComplete(c *gc.C) {
	results, err := s.uniterSuite.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, ([]*state.ActionResult)(nil))

	action, err := s.uniterSuite.wordpressUnit.AddAction("gabloxi", nil)
	c.Assert(err, gc.IsNil)

	err = s.uniter.ActionComplete(action.ActionTag(), "it worked!")
	c.Assert(err, gc.IsNil)

	results, err = s.uniterSuite.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	c.Assert(results[0].Output(), gc.Equals, "it worked!")
	c.Assert(results[0].ActionName(), gc.Equals, "gabloxi")
}

func (s *actionSuite) TestActionFail(c *gc.C) {
	results, err := s.uniterSuite.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, ([]*state.ActionResult)(nil))

	action, err := s.uniterSuite.wordpressUnit.AddAction("beebz", nil)
	c.Assert(err, gc.IsNil)

	err = s.uniter.ActionFail(action.ActionTag(), "it failed!")
	c.Assert(err, gc.IsNil)

	results, err = s.uniterSuite.wordpressUnit.ActionResults()
	c.Assert(err, gc.IsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	c.Assert(results[0].Output(), gc.Equals, "it failed!")
	c.Assert(results[0].ActionName(), gc.Equals, "beebz")
}
