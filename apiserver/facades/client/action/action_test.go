// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/actions"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper

	action     *action.ActionAPI
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	charm         *state.Charm
	machine0      *state.Machine
	machine1      *state.Machine
	dummy         *state.Application
	wordpress     *state.Application
	mysql         *state.Application
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

type actionSuite struct {
	baseSuite
}

var _ = gc.Suite(&actionSuite{})

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.OpenControllerModelAPI(c))
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	var err error
	s.action, err = action.NewActionAPI(s.ControllerModel(c).State(), s.resources, s.authorizer, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.charm = f.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
	})

	s.dummy = f.MakeApplication(c, &factory.ApplicationParams{
		Name: "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "dummy",
		}),
	})
	s.wordpress = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.charm,
	})
	s.machine0 = f.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.wordpressUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	mysqlCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})
	s.machine1 = f.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.mysqlUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
}

func (s *actionSuite) TestActions(c *gc.C) {
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"foo": 1, "bar": "please"}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"baz": true}},
		}}

	r, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Actions, gc.HasLen, len(arg.Actions))

	// There's only one operation created.
	operations, err := s.ControllerModel(c).AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Summary(), gc.Equals, "fakeaction run on unit-wordpress-0,unit-mysql-0,unit-wordpress-0,unit-mysql-0")

	emptyActionTag := names.ActionTag{}
	for i, got := range r.Actions {
		c.Assert(got.Action, gc.NotNil)
		c.Logf("check index %d (%s: %s)", i, got.Action.Tag, arg.Actions[i].Name)
		c.Assert(got.Error, gc.Equals, (*params.Error)(nil))
		c.Assert(got.Action, gc.Not(gc.Equals), (*params.Action)(nil))
		c.Assert(got.Action.Tag, gc.Not(gc.Equals), emptyActionTag)
		c.Assert(got.Action.Name, gc.Equals, arg.Actions[i].Name)
		c.Assert(got.Action.Receiver, gc.Equals, arg.Actions[i].Receiver)
		c.Assert(got.Action.Parameters, gc.DeepEquals, arg.Actions[i].Parameters)
		c.Assert(got.Status, gc.Equals, params.ActionPending)
		c.Assert(got.Message, gc.Equals, "")
		c.Assert(got.Output, gc.IsNil)
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

	results, err := s.action.EnqueueOperation(tests)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Actions, gc.HasLen, 4)
	for _, res := range results.Actions {
		c.Assert(res.Error, gc.IsNil)
	}

	// blocking changes should have no effect
	s.BlockAllChanges(c, "Cancel")

	// Cancel Some.
	arg := params.Entities{
		Entities: []params.Entity{
			// "wp-two"
			{Tag: results.Actions[1].Action.Tag},
			// "my-one"
			{Tag: results.Actions[2].Action.Tag},
		}}
	cancelled, err := s.action.Cancel(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cancelled.Results, gc.HasLen, 2)

	// Assert the Actions are all in the expected state.
	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Units: []string{
			s.wordpressUnit.Name(),
			s.mysqlUnit.Name(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)

	resultActions := operations.Results[0].Actions
	c.Assert(resultActions, gc.HasLen, 4)
	c.Assert(resultActions[0].Action.Name, gc.Equals, "fakeaction")
	c.Assert(resultActions[0].Status, gc.Equals, params.ActionPending)
	c.Assert(resultActions[1].Action.Name, gc.Equals, "fakeaction")
	c.Assert(resultActions[1].Status, gc.Equals, params.ActionCancelled)
	c.Assert(resultActions[2].Action.Name, gc.Equals, "fakeaction")
	c.Assert(resultActions[2].Status, gc.Equals, params.ActionCancelled)
	c.Assert(resultActions[3].Action.Name, gc.Equals, "fakeaction")
	c.Assert(resultActions[3].Status, gc.Equals, params.ActionPending)
}

func (s *actionSuite) TestAbort(c *gc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	// Add Actions.
	tests := params.Actions{
		Actions: []params.Action{{
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}},
	}

	results, err := s.action.EnqueueOperation(tests)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Actions, gc.HasLen, 1)
	c.Assert(results.Actions[0].Error, gc.IsNil)

	actions, err = s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 1)

	_, err = actions[0].Begin()
	c.Assert(err, jc.ErrorIsNil)

	// blocking changes should have no effect
	s.BlockAllChanges(c, "Cancel")

	// Cancel Some.
	arg := params.Entities{
		Entities: []params.Entity{
			// "wp-one"
			{Tag: results.Actions[0].Action.Tag},
		}}
	cancelled, err := s.action.Cancel(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cancelled.Results, gc.HasLen, 1)
	c.Assert(cancelled.Results[0].Action.Name, gc.Equals, "fakeaction")
	c.Assert(cancelled.Results[0].Status, gc.Equals, params.ActionAborting)

	// Assert the Actions are all in the expected state.
	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Units: []string{s.wordpressUnit.Name()},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)

	wpActions := operations.Results[0].Actions
	c.Assert(wpActions, gc.HasLen, 1)
	c.Assert(wpActions[0].Action.Name, gc.Equals, "fakeaction")
	c.Assert(wpActions[0].Status, gc.Equals, params.ActionAborting)
}

func (s *actionSuite) TestApplicationsCharmsActions(c *gc.C) {
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
		applicationNames []string
		expectedResults  params.ApplicationsCharmActionsResults
	}{{
		applicationNames: []string{"dummy"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("dummy").String(),
					Actions: map[string]params.ActionSpec{
						"snapshot": {
							Description: "Take a snapshot of the database.",
							Params:      actionSchemas["snapshot"],
						},
					},
				},
			},
		},
	}, {
		applicationNames: []string{"wordpress"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("wordpress").String(),
					Actions: map[string]params.ActionSpec{
						"fakeaction": {
							Description: "No description",
							Params:      actionSchemas["fakeaction"],
						},
					},
				},
			},
		},
	}, {
		applicationNames: []string{"nonsense"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("nonsense").String(),
					Error: &params.Error{
						Message: `application "nonsense" not found`,
						Code:    "not found",
					},
				},
			},
		},
	}}

	for i, t := range tests {
		c.Logf("test %d: applications: %#v", i, t.applicationNames)

		svcTags := params.Entities{
			Entities: make([]params.Entity, len(t.applicationNames)),
		}

		for j, app := range t.applicationNames {
			svcTag := names.NewApplicationTag(app)
			svcTags.Entities[j] = params.Entity{Tag: svcTag.String()}
		}

		results, err := s.action.ApplicationsCharmsActions(context.Background(), svcTags)
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

func (s *actionSuite) TestWatchActionProgress(c *gc.C) {
	unit, err := s.ControllerModel(c).State().Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	assertReadyToTest(c, unit)

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	added, err := s.ControllerModel(c).AddAction(unit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.action.WatchActionsProgress(
		context.Background(),
		params.Entities{Entities: []params.Entity{{Tag: "action-2"}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w.Results, gc.HasLen, 1)
	c.Assert(w.Results[0].Error, gc.IsNil)
	c.Assert(w.Results[0].Changes, gc.HasLen, 0)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Log a message and check the watcher result.
	added, err = added.Begin()
	c.Assert(err, jc.ErrorIsNil)
	err = added.Log("hello")
	c.Assert(err, jc.ErrorIsNil)

	a, err := s.ControllerModel(c).Action("2")
	c.Assert(err, jc.ErrorIsNil)
	logged := a.Messages()
	c.Assert(logged, gc.HasLen, 1)
	expected, err := json.Marshal(actions.ActionMessage{
		Message:   logged[0].Message(),
		Timestamp: logged[0].Timestamp(),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(string(expected))
	wc.AssertNoChange()
}
