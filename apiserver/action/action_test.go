// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"fmt"
	"testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/action"
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

type actionSuite struct {
	jujutesting.JujuConnSuite

	action     *action.ActionAPI
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	charm         *state.Charm
	machine0      *state.Machine
	machine1      *state.Machine
	dummy         *state.Service
	wordpress     *state.Service
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.action, err = action.NewActionAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	factory := jujuFactory.NewFactory(s.State)

	s.charm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
	})

	s.dummy = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name: "dummy",
		Charm: factory.MakeCharm(c, &jujuFactory.CharmParams{
			Name: "dummy",
		}),
		Creator: s.AdminUserTag(c),
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

func (s *actionSuite) TestActions(c *gc.C) {
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"foo": 1, "bar": "please"}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"baz": true}},
		}}

	r, err := s.action.Enqueue(arg)
	c.Assert(err, gc.Equals, nil)
	c.Assert(r.Results, gc.HasLen, len(arg.Actions))

	entities := make([]params.Entity, len(r.Results))
	for i, result := range r.Results {
		entities[i] = params.Entity{Tag: result.Action.Tag}
	}

	actions, err := s.action.Actions(params.Entities{Entities: entities})
	c.Assert(err, gc.Equals, nil)

	c.Assert(len(actions.Results), gc.Equals, len(entities))
	for i, got := range actions.Results {
		c.Logf("check index %d (%s: %s)", i, entities[i].Tag, arg.Actions[i].Name)
		c.Assert(got.Error, gc.Equals, (*params.Error)(nil))
		c.Assert(got.Action, gc.Not(gc.Equals), (*params.Action)(nil))
		c.Assert(got.Action.Tag, gc.Equals, entities[i].Tag)
		c.Assert(got.Action.Name, gc.Equals, arg.Actions[i].Name)
		c.Assert(got.Action.Receiver, gc.Equals, arg.Actions[i].Receiver)
		c.Assert(got.Action.Parameters, gc.DeepEquals, arg.Actions[i].Parameters)
		c.Assert(got.Status, gc.Equals, params.ActionPending)
		c.Assert(got.Message, gc.Equals, "")
		c.Assert(got.Output, gc.DeepEquals, map[string]interface{}{})
	}
}

func (s *actionSuite) TestFindActionTagsByPrefix(c *gc.C) {
	// NOTE: full testing with multiple matches has been moved to state package.
	arg := params.Actions{Actions: []params.Action{{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}}}}
	r, err := s.action.Enqueue(arg)
	c.Assert(err, gc.Equals, nil)
	c.Assert(r.Results, gc.HasLen, len(arg.Actions))

	actionTag, err := names.ParseActionTag(r.Results[0].Action.Tag)
	c.Assert(err, gc.Equals, nil)
	prefix := actionTag.Id()[:7]
	tags, err := s.action.FindActionTagsByPrefix(params.FindTags{Prefixes: []string{prefix}})
	c.Assert(err, gc.Equals, nil)

	entities, ok := tags.Matches[prefix]
	c.Assert(ok, gc.Equals, true)
	c.Assert(len(entities), gc.Equals, 1)
	c.Assert(entities[0].Tag, gc.Equals, actionTag.String())
}

func (s *actionSuite) TestEnqueue(c *gc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Make sure no Actions already exist on mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Add Actions.
	expectedName := "fakeaction"
	expectedParameters := map[string]interface{}{"kan jy nie": "verstaand"}
	arg := params.Actions{
		Actions: []params.Action{
			// No receiver.
			{Name: "fakeaction"},
			// Good.
			{Receiver: s.wordpressUnit.Tag().String(), Name: expectedName, Parameters: expectedParameters},
			// Service tag instead of Unit tag.
			{Receiver: s.wordpress.Tag().String(), Name: "fakeaction"},
			// Missing name.
			{Receiver: s.mysqlUnit.Tag().String(), Parameters: expectedParameters},
		},
	}
	res, err := s.action.Enqueue(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 4)

	expectedError := &params.Error{Message: "id not found", Code: "not found"}
	emptyActionTag := names.ActionTag{}
	c.Assert(res.Results[0].Error, gc.DeepEquals, expectedError)
	c.Assert(res.Results[0].Action, gc.IsNil)

	c.Assert(res.Results[1].Error, gc.IsNil)
	c.Assert(res.Results[1].Action, gc.NotNil)
	c.Assert(res.Results[1].Action.Receiver, gc.Equals, s.wordpressUnit.Tag().String())
	c.Assert(res.Results[1].Action.Tag, gc.Not(gc.Equals), emptyActionTag)

	c.Assert(res.Results[2].Error, gc.DeepEquals, expectedError)
	c.Assert(res.Results[2].Action, gc.IsNil)

	c.Assert(res.Results[3].Error, gc.ErrorMatches, "no action name given")
	c.Assert(res.Results[3].Action, gc.IsNil)

	// Make sure an Action was enqueued for the wordpress Unit.
	actions, err = s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 1)
	c.Assert(actions[0].Name(), gc.Equals, expectedName)
	c.Assert(actions[0].Parameters(), gc.DeepEquals, expectedParameters)
	c.Assert(actions[0].Receiver(), gc.Equals, s.wordpressUnit.Name())

	// Make sure an Action was not enqueued for the mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)
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

var testCases = []testCase{{
	Groups: []receiverGroup{
		{
			ExpectedError: &params.Error{Message: "id not found", Code: "not found"},
			Receiver:      names.NewServiceTag("wordpress"),
			Actions:       []testCaseAction{},
		}, {
			Receiver: names.NewUnitTag("wordpress/0"),
			Actions: []testCaseAction{
				{"fakeaction", map[string]interface{}{}, false},
				{"fakeaction", map[string]interface{}{"asdf": 3}, true},
				{"fakeaction", map[string]interface{}{"qwer": "ty"}, false},
			},
		}, {
			Receiver: names.NewUnitTag("mysql/0"),
			Actions: []testCaseAction{
				{"fakeaction", map[string]interface{}{"zxcv": false}, false},
				{"fakeaction", map[string]interface{}{}, true},
			},
		},
	},
}}

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

			// add each action from the test case.
			for j, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				// make expectation
				exp := &cur.Actions[j]
				exp.Action = &params.Action{
					Tag:        added.ActionTag().String(),
					Name:       action.Name,
					Parameters: action.Parameters,
				}
				exp.Status = params.ActionPending
				exp.Output = map[string]interface{}{}

				if action.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					fa, err := added.Finish(state.ActionResults{Status: status, Results: output, Message: message})
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(fa.Status(), gc.Equals, state.ActionCompleted)

					exp.Status = string(status)
					exp.Message = message
					exp.Output = output
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

			// add each action from the test case.
			for _, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if action.Execute {
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
							Name:       action.Name,
							Parameters: action.Parameters,
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

			// add each action from the test case.
			for _, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if action.Execute {
					started, err := added.Begin()
					c.Assert(err, jc.ErrorIsNil)
					c.Assert(started.Status(), gc.Equals, state.ActionRunning)

					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag().String(),
							Name:       action.Name,
							Parameters: action.Parameters,
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

			// add each action from the test case.
			for _, action := range group.Actions {
				// add action.
				added, err := unit.AddAction(action.Name, action.Parameters)
				c.Assert(err, jc.ErrorIsNil)

				if action.Execute {
					status := state.ActionCompleted
					output := map[string]interface{}{"output": "blah, blah, blah"}
					message := "success"

					_, err = added.Finish(state.ActionResults{Status: status, Results: output, Message: message})
					c.Assert(err, jc.ErrorIsNil)

					// add expectation
					exp := params.ActionResult{
						Action: &params.Action{
							Tag:        added.ActionTag().String(),
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
		actionList, err := s.action.ListCompleted(arg)
		c.Assert(err, jc.ErrorIsNil)
		assertSame(c, actionList, expected)
	}
}

func (s *actionSuite) TestCancel(c *gc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Make sure no Actions already exist on mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Add Actions.
	tests := params.Actions{
		Actions: []params.Action{{
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.mysqlUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.mysqlUnit.Tag().String(),
			Name:     "fakeaction",
		}},
	}

	results, err := s.action.Enqueue(tests)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 4)
	for _, res := range results.Results {
		c.Assert(res.Error, gc.IsNil)
	}

	// Cancel Some.
	arg := params.Entities{
		Entities: []params.Entity{
			// "wp-two"
			{Tag: results.Results[1].Action.Tag},
			// "my-one"
			{Tag: results.Results[2].Action.Tag},
		}}
	results, err = s.action.Cancel(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)

	// Assert the Actions are all in the expected state.
	tags := params.Entities{Entities: []params.Entity{{Tag: s.wordpressUnit.Tag().String()}, {Tag: s.mysqlUnit.Tag().String()}}}
	obtained, err := s.action.ListAll(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.Actions, gc.HasLen, 2)

	wpActions := obtained.Actions[0].Actions
	c.Assert(wpActions, gc.HasLen, 2)
	c.Assert(wpActions[0].Action.Name, gc.Equals, "fakeaction")
	c.Assert(wpActions[0].Status, gc.Equals, params.ActionPending)
	c.Assert(wpActions[1].Action.Name, gc.Equals, "fakeaction")
	c.Assert(wpActions[1].Status, gc.Equals, params.ActionCancelled)

	myActions := obtained.Actions[1].Actions
	c.Assert(myActions, gc.HasLen, 2)
	c.Assert(myActions[0].Action.Name, gc.Equals, "fakeaction")
	c.Assert(myActions[0].Status, gc.Equals, params.ActionPending)
	c.Assert(myActions[1].Action.Name, gc.Equals, "fakeaction")
	c.Assert(myActions[1].Status, gc.Equals, params.ActionCancelled)
}

func (s *actionSuite) TestServicesCharmActions(c *gc.C) {
	actionSchemas := map[string]map[string]interface{}{
		"snapshot": {
			"type":        "object",
			"title":       "snapshot",
			"description": "Take a snapshot of the database.",
			"properties": map[string]interface{}{
				"outfile": map[string]interface{}{
					"description": "The file to write out to.",
					"type":        "string",
					"default":     "foo.bz2",
				},
			},
		},
		"fakeaction": {
			"type":        "object",
			"title":       "fakeaction",
			"description": "No description",
			"properties":  map[string]interface{}{},
		},
	}
	tests := []struct {
		serviceNames    []string
		expectedResults params.ServicesCharmActionsResults
	}{{
		serviceNames: []string{"dummy"},
		expectedResults: params.ServicesCharmActionsResults{
			Results: []params.ServiceCharmActionsResult{
				{
					ServiceTag: names.NewServiceTag("dummy").String(),
					Actions: &charm.Actions{
						ActionSpecs: map[string]charm.ActionSpec{
							"snapshot": {
								Description: "Take a snapshot of the database.",
								Params:      actionSchemas["snapshot"],
							},
						},
					},
				},
			},
		},
	}, {
		serviceNames: []string{"wordpress"},
		expectedResults: params.ServicesCharmActionsResults{
			Results: []params.ServiceCharmActionsResult{
				{
					ServiceTag: names.NewServiceTag("wordpress").String(),
					Actions: &charm.Actions{
						ActionSpecs: map[string]charm.ActionSpec{
							"fakeaction": {
								Description: "No description",
								Params:      actionSchemas["fakeaction"],
							},
						},
					},
				},
			},
		},
	}, {
		serviceNames: []string{"nonsense"},
		expectedResults: params.ServicesCharmActionsResults{
			Results: []params.ServiceCharmActionsResult{
				{
					ServiceTag: names.NewServiceTag("nonsense").String(),
					Error: &params.Error{
						Message: `service "nonsense" not found`,
						Code:    "not found",
					},
				},
			},
		},
	}}

	for i, t := range tests {
		c.Logf("test %d: services: %#v", i, t.serviceNames)

		svcTags := params.Entities{
			Entities: make([]params.Entity, len(t.serviceNames)),
		}

		for j, svc := range t.serviceNames {
			svcTag := names.NewServiceTag(svc)
			svcTags.Entities[j] = params.Entity{Tag: svcTag.String()}
		}

		results, err := s.action.ServicesCharmActions(svcTags)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(results.Results, jc.DeepEquals, t.expectedResults.Results)
	}
}

func assertReadyToTest(c *gc.C, receiver state.ActionReceiver) {
	// make sure there are no actions on the receiver already.
	actions, err := receiver.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// make sure there are no actions pending already.
	actions, err = receiver.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// make sure there are no actions running already.
	actions, err = receiver.RunningActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// make sure there are no actions completed already.
	actions, err = receiver.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)
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
