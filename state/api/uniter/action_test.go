// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
	gc "launchpad.net/gocheck"
)

type actionSuite struct {
	uniterSuite
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) TestAction(c *gc.C) {

	var actionTests = []struct {
		description string
		action      params.Action
	}{{
		description: "A simple Action.",
		action: params.Action{
			Name: "snapshot",
			Params: map[string]interface{}{
				"outfile": "foo.txt",
			},
		},
	}, {
		description: "An Action with nested parameters.",
		action: params.Action{
			Name: "backup",
			Params: map[string]interface{}{
				"outfile": "foo.bz2",
				"compression": map[string]interface{}{
					"kind": "bzip",
					// BUG(?): this fails with int quality
					"quality": float64(5.0),
				},
			},
		},
	}}

	for i, actionTest := range actionTests {
		c.Logf("test %d: %s", i, actionTest.description)
		actionId, err := s.uniterSuite.wordpressUnit.AddAction(
			actionTest.action.Name,
			actionTest.action.Params)
		c.Assert(err, gc.IsNil)

		retrievedAction, err := s.uniter.Action(actionId)
		c.Assert(err, gc.IsNil)

		c.Assert(retrievedAction.Name(), gc.DeepEquals, actionTest.action.Name)
		c.Assert(retrievedAction.Params(), gc.DeepEquals, actionTest.action.Params)
	}
}

func (s *actionSuite) TestActionNotFound(c *gc.C) {
	actionId := "foo"
	_, err := s.uniter.Action(actionId)
	c.Assert(err, gc.NotNil)
}

func (s *actionSuite) TestNewActionAndAccessors(c *gc.C) {
	testAction, err := uniter.NewAction("snapshot", map[string]interface{}{
		"outfile": "foo.txt"})
	c.Assert(err, gc.IsNil)
	testName := testAction.Name()
	testParams := testAction.Params()
	c.Assert(testName, gc.Equals, "snapshot")
	c.Assert(testParams, gc.DeepEquals, map[string]interface{}{
		"outfile": "foo.txt"})
}
