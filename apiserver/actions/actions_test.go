// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"fmt"
	"testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/actions"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuFactory "github.com/juju/juju/testing/factory"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type actionsSuite struct {
	jujutesting.JujuConnSuite

	actions    *actions.ActionsAPI
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	charm         *state.Charm
	machine0      *state.Machine
	machine1      *state.Machine
	wordpress     *state.Service
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.actions, err = actions.NewActionsAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)

	factory := jujuFactory.NewFactory(s.State)

	s.charm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
	})

	s.wordpress = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "wordpress",
		Charm:   s.charm,
		Creator: s.AdminUserTag(c),
	})
	s.machine0 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	})
	s.wordpressUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.wordpress,
		Machine: s.machine0,
	})

	mysqlCharm := factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "mysql",
	})
	s.mysql = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "mysql",
		Charm:   mysqlCharm,
		Creator: s.AdminUserTag(c),
	})
	s.machine1 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	s.mysqlUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.mysql,
		Machine: s.machine1,
	})
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
}

func (s *actionsSuite) TestEnqueue(c *gc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Make sure no Actions already exist on mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Add Actions.
	expectedName := "bar"
	expectedParameters := map[string]interface{}{"kan jy nie": "verstaand"}
	arg := params.Actions{
		Actions: []params.Action{
			// No receiver.
			{Name: "foo"},
			// Good.
			{Receiver: s.wordpressUnit.Tag(), Name: expectedName, Parameters: expectedParameters},
			// Service tag instead of Unit tag.
			{Receiver: s.wordpress.Tag(), Name: "baz"},
			// TODO(jcw4) notice no Name.
			{Receiver: s.mysqlUnit.Tag(), Parameters: expectedParameters},
		},
	}
	res, err := s.actions.Enqueue(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 4)

	expectedError := &params.Error{Message: "id not found", Code: "not found"}
	emptyActionTag := names.ActionTag{}
	c.Assert(res.Results[0].Error, gc.DeepEquals, expectedError)
	c.Assert(res.Results[0].Action, gc.IsNil)

	c.Assert(res.Results[1].Error, gc.IsNil)
	c.Assert(res.Results[1].Action, gc.NotNil)
	c.Assert(res.Results[1].Action.Receiver, gc.Equals, s.wordpressUnit.Tag())
	c.Assert(res.Results[1].Action.Tag, gc.Not(gc.Equals), emptyActionTag)

	c.Assert(res.Results[2].Error, gc.DeepEquals, expectedError)
	c.Assert(res.Results[2].Action, gc.IsNil)

	// TODO(jcw4) shouldn't Action Names be required?
	c.Assert(res.Results[3].Error, gc.IsNil)
	c.Assert(res.Results[3].Action, gc.NotNil)
	c.Assert(res.Results[3].Action.Receiver, gc.Equals, s.mysqlUnit.Tag())
	c.Assert(res.Results[3].Action.Tag, gc.Not(gc.Equals), emptyActionTag)

	// Make sure an Action was enqueued for the wordpress Unit.
	actions, err = s.wordpressUnit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(actions, gc.HasLen, 1)
	c.Assert(actions[0].Name(), gc.Equals, expectedName)
	c.Assert(actions[0].Parameters(), gc.DeepEquals, expectedParameters)
	c.Assert(actions[0].Receiver(), gc.Equals, s.wordpressUnit.Name())

	// Make sure an Action was enqueued for the mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, gc.IsNil)
	c.Assert(actions, gc.HasLen, 1)
	// TODO(jcw4) notice Action Name empty.
	c.Assert(actions[0].Name(), gc.Equals, "")
	c.Assert(actions[0].Parameters(), gc.DeepEquals, expectedParameters)
	c.Assert(actions[0].Receiver(), gc.Equals, s.mysqlUnit.Name())
}

type testCaseAction struct {
	Name       string
	Parameters map[string]interface{}
	Execute    bool
}

type receiverGroup struct {
	ExpectedError *params.Error
	Receiver      names.Tag
	Actions       []testCaseAction
}

type testCase struct {
	Groups []receiverGroup
}

var listTestCases = []testCase{{
	Groups: []receiverGroup{
		{
			ExpectedError: &params.Error{Message: "id not found", Code: "not found"},
			Receiver:      names.NewServiceTag("wordpress"),
			Actions:       []testCaseAction{},
		}, {
			Receiver: names.NewUnitTag("wordpress/0"),
			Actions: []testCaseAction{
				{"foo", map[string]interface{}{}, false},
				{"bar", map[string]interface{}{"asdf": 3}, true},
				{"baz", map[string]interface{}{"qwer": "ty"}, false},
			},
		}, {
			Receiver: names.NewUnitTag("mysql/0"),
			Actions: []testCaseAction{
				{"oof", map[string]interface{}{"zxcv": false}, false},
				{"rab", map[string]interface{}{}, true},
			},
		},
	},
}}

func (s *actionsSuite) TestListAll(c *gc.C) {
	for _, testCase := range listTestCases {
		// set up query args
		arg := params.Tags{Tags: make([]names.Tag, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Tags[i] = group.Receiver

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver
			cur.Actions = make([]params.ActionResult, len(group.Actions))

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, gc.IsNil)

			// make sure there are no actions queued up already.
			actions, err := unit.Actions()
			c.Assert(err, gc.IsNil)
			c.Assert(actions, gc.HasLen, 0)

			// make sure there are no action results queued up already.
			results, err := unit.ActionResults()
			c.Assert(err, gc.IsNil)
			c.Assert(results, gc.HasLen, 0)

			// add each action from the test case.
			for j, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, gc.IsNil)

				// make expectation
				exp := &cur.Actions[j]
				exp.Action = &params.Action{
					Tag:        added.ActionTag(),
					Name:       action.Name,
					Parameters: action.Parameters,
				}
				exp.Status = "pending"

				if action.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					err = added.Finish(state.ActionResults{status, output, message})
					c.Assert(err, gc.IsNil)

					exp.Status = string(status)
					exp.Message = message
					exp.Output = output
				}
			}
		}

		// validate assumptions.
		actionList, err := s.actions.ListAll(arg)
		c.Assert(err, gc.IsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionsSuite) TestListPending(c *gc.C) {
	for _, testCase := range listTestCases {
		// set up query args
		arg := params.Tags{Tags: make([]names.Tag, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Tags[i] = group.Receiver

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver
			cur.Actions = []params.ActionResult{}

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, gc.IsNil)

			// make sure there are no actions queued up already.
			actions, err := unit.Actions()
			c.Assert(err, gc.IsNil)
			c.Assert(actions, gc.HasLen, 0)

			// make sure there are no action results queued up already.
			results, err := unit.ActionResults()
			c.Assert(err, gc.IsNil)
			c.Assert(results, gc.HasLen, 0)

			// add each action from the test case.
			for _, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, gc.IsNil)

				if action.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					err = added.Finish(state.ActionResults{status, output, message})
					c.Assert(err, gc.IsNil)
				} else {
					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag(),
							Name:       action.Name,
							Parameters: action.Parameters,
						},
						Status: "pending",
					}
					cur.Actions = append(cur.Actions, exp)
				}
			}
		}

		// validate assumptions.
		actionList, err := s.actions.ListPending(arg)
		c.Assert(err, gc.IsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionsSuite) TestListCompleted(c *gc.C) {
	for _, testCase := range listTestCases {
		// set up query args
		arg := params.Tags{Tags: make([]names.Tag, len(testCase.Groups))}

		// prepare state, and set up expectations.
		expected := params.ActionsByReceivers{Actions: make([]params.ActionsByReceiver, len(testCase.Groups))}
		for i, group := range testCase.Groups {
			arg.Tags[i] = group.Receiver

			cur := &expected.Actions[i]
			cur.Error = group.ExpectedError

			// short circuit and bail if the ActionReceiver isn't a Unit.
			if _, ok := group.Receiver.(names.UnitTag); !ok {
				continue
			}

			cur.Receiver = group.Receiver
			cur.Actions = []params.ActionResult{}

			// get Unit (ActionReceiver) for this Pair in the test case.
			unit, err := s.State.Unit(group.Receiver.Id())
			c.Assert(err, gc.IsNil)

			// make sure there are no actions queued up already.
			actions, err := unit.Actions()
			c.Assert(err, gc.IsNil)
			c.Assert(actions, gc.HasLen, 0)

			// make sure there are no action results queued up already.
			results, err := unit.ActionResults()
			c.Assert(err, gc.IsNil)
			c.Assert(results, gc.HasLen, 0)

			// add each action from the test case.
			for _, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, gc.IsNil)

				if action.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					err = added.Finish(state.ActionResults{status, output, message})
					c.Assert(err, gc.IsNil)

					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag(),
							Name:       action.Name,
							Parameters: action.Parameters,
						},
						Status:  string(status),
						Message: message,
						Output:  output,
					}
					cur.Actions = append(cur.Actions, exp)
				}
			}
		}

		// validate assumptions.
		actionList, err := s.actions.ListCompleted(arg)
		c.Assert(err, gc.IsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionsSuite) TestCancel(c *gc.C) {
	// TODO(jcw4) implement
	c.Skip("Cancel not yet implemented")
}

func assertSame(c *gc.C, got, expected params.ActionsByReceivers) {
	c.Assert(got.Actions, gc.HasLen, len(expected.Actions))
	for i, g1 := range got.Actions {
		e1 := expected.Actions[i]
		c.Assert(g1.Error, gc.DeepEquals, e1.Error)
		c.Assert(g1.Receiver, gc.DeepEquals, e1.Receiver)
		c.Assert(toStrings(g1.Actions), jc.SameContents, toStrings(e1.Actions))
	}
}

func toStrings(items []params.ActionResult) []string {
	ret := make([]string, len(items))
	for i, a := range items {
		ret[i] = stringify(a)
	}
	return ret
}

func stringify(r params.ActionResult) string {
	a := r.Action
	if a == nil {
		a = &params.Action{}
	}
	return fmt.Sprintf("%s-%s-%#v-%s-%s-%#v", a.Tag, a.Name, a.Parameters, r.Status, r.Message, r.Output)
}
