// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func (s *actionSuite) TestListAll(c *gc.C) {
	for _, testCase := range testCases {
		// set up query args
		arg := params.Entities{Entities: make([]params.Entity, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Entities[i] = params.Entity{Tag: group.Receiver.String()}

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver.String()
			cur.Actions = make([]params.ActionResult, len(group.Actions))

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, jc.ErrorIsNil)
			assertReadyToTest(c, unit)

			operationID, err := s.Model.EnqueueOperation("a test")
			c.Assert(err, jc.ErrorIsNil)
			// add each action from the test case.
			for j, act := range group.Actions {
				// add action.
				added, err := unit.AddAction(operationID, act.Name, act.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				// make expectation
				exp := &cur.Actions[j]
				exp.Action = &params.Action{
					Tag:        added.ActionTag().String(),
					Name:       act.Name,
					Parameters: act.Parameters,
				}
				exp.Status = params.ActionPending
				exp.Output = map[string]interface{}{}

				if act.Execute {
					added, err = added.Begin()
					c.Assert(err, jc.ErrorIsNil)
					err = added.Log("hello")
					c.Assert(err, jc.ErrorIsNil)
					status := state.ActionCompleted
					output := map[string]interface{}{
						"output":         "blah, blah, blah",
						"Stdout":         "out",
						"StdoutEncoding": "utf-8",
						"Stderr":         "err",
						"StderrEncoding": "utf-8",
						"Code":           "1",
					}
					message := "success"

					fa, err := added.Finish(state.ActionResults{Status: status, Results: output, Message: message})
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(fa.Status(), gc.Equals, state.ActionCompleted)

					exp.Status = string(status)
					exp.Message = message
					exp.Output = map[string]interface{}{
						"output":          "blah, blah, blah",
						"stdout":          "out",
						"stdout-encoding": "utf-8",
						"stderr":          "err",
						"stderr-encoding": "utf-8",
						"return-code":     1,
					}
					exp.Log = []params.ActionMessage{{Message: "hello"}}
				}
			}
		}

		// validate assumptions.
		actionList, err := s.action.ListAll(arg)
		c.Assert(err, jc.ErrorIsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionSuite) TestListPending(c *gc.C) {
	for _, testCase := range testCases {
		// set up query args
		arg := params.Entities{Entities: make([]params.Entity, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Entities[i] = params.Entity{Tag: group.Receiver.String()}

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver.String()
			cur.Actions = []params.ActionResult{}

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, jc.ErrorIsNil)
			assertReadyToTest(c, unit)

			operationID, err := s.Model.EnqueueOperation("a test")
			c.Assert(err, jc.ErrorIsNil)
			// add each action from the test case.
			for _, act := range group.Actions {
				// add action.
				added, err := unit.AddAction(operationID, act.Name, act.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if act.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					fa, err := added.Finish(state.ActionResults{Status: status, Results: output, Message: message})
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(fa.Status(), gc.Equals, state.ActionCompleted)
				} else {
					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag().String(),
							Name:       act.Name,
							Parameters: act.Parameters,
						},
						Status: params.ActionPending,
						Output: map[string]interface{}{},
					}
					cur.Actions = append(cur.Actions, exp)
				}
			}
		}

		// validate assumptions.
		actionList, err := s.action.ListPending(arg)
		c.Assert(err, jc.ErrorIsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionSuite) TestListRunning(c *gc.C) {
	for _, testCase := range testCases {
		// set up query args
		arg := params.Entities{Entities: make([]params.Entity, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Entities[i] = params.Entity{Tag: group.Receiver.String()}

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver.String()
			cur.Actions = []params.ActionResult{}

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, jc.ErrorIsNil)
			assertReadyToTest(c, unit)

			operationID, err := s.Model.EnqueueOperation("a test")
			c.Assert(err, jc.ErrorIsNil)
			// add each action from the test case.
			for _, act := range group.Actions {
				// add action.
				added, err := unit.AddAction(operationID, act.Name, act.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if act.Execute {
					started, err := added.Begin()
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(started.Status(), gc.Equals, state.ActionRunning)

					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag().String(),
							Name:       act.Name,
							Parameters: act.Parameters,
						},
						Status: params.ActionRunning,
						Output: map[string]interface{}{},
					}
					cur.Actions = append(cur.Actions, exp)
				}
			}
		}

		// validate assumptions.
		actionList, err := s.action.ListRunning(arg)
		c.Assert(err, jc.ErrorIsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionSuite) TestListCompleted(c *gc.C) {
	for _, testCase := range testCases {
		// set up query args
		arg := params.Entities{Entities: make([]params.Entity, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Entities[i] = params.Entity{Tag: group.Receiver.String()}

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver.String()
			cur.Actions = []params.ActionResult{}

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, jc.ErrorIsNil)
			assertReadyToTest(c, unit)

			operationID, err := s.Model.EnqueueOperation("a test")
			c.Assert(err, jc.ErrorIsNil)
			// add each action from the test case.
			for _, act := range group.Actions {
				// add action.
				added, err := unit.AddAction(operationID, act.Name, act.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if act.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{
						"output":         "blah, blah, blah",
						"Stdout":         "out",
						"StdoutEncoding": "utf-8",
						"Stderr":         "err",
						"StderrEncoding": "utf-8",
						"Code":           "1",
					}
					message := "success"

					_, err = added.Finish(state.ActionResults{Status: status, Results: output, Message: message})
					c.Assert(err, jc.ErrorIsNil)

					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag().String(),
							Name:       act.Name,
							Parameters: act.Parameters,
						},
						Status:  string(status),
						Message: message,
						Output: map[string]interface{}{
							"output":          "blah, blah, blah",
							"stdout":          "out",
							"stdout-encoding": "utf-8",
							"stderr":          "err",
							"stderr-encoding": "utf-8",
							"return-code":     1,
						},
					}
					cur.Actions = append(cur.Actions, exp)
				}
			}
		}

		// validate assumptions.
		actionList, err := s.action.ListCompleted(arg)
		c.Assert(err, jc.ErrorIsNil)
		assertSame(c, actionList, expected)
	}
}
