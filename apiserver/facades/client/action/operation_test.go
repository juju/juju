// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"fmt"
	"strconv"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type operationSuite struct {
	baseSuite
}

var _ = gc.Suite(&operationSuite{})

func (s *operationSuite) setupOperations(c *gc.C) {
	s.toSupportNewActionID(c)

	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "anotherfakeaction", Parameters: map[string]interface{}{}},
		}}

	r, err := s.action.Enqueue(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, len(arg.Actions))

	// There's only one operation created.
	ops, err := s.Model.AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)
	operationID, err := strconv.Atoi(ops[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	a, err := s.Model.Action(strconv.Itoa(operationID + 1))
	c.Assert(err, jc.ErrorIsNil)
	_, err = a.Begin()
	c.Assert(err, jc.ErrorIsNil)
	a, err = s.Model.Action(strconv.Itoa(operationID + 2))
	c.Assert(err, jc.ErrorIsNil)
	_, err = a.Finish(state.ActionResults{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *operationSuite) TestEnqueueOperation(c *gc.C) {
	// Ensure wordpress unit is the leader.
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim("wordpress", "wordpress/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

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
			// Application tag instead of Unit tag.
			{Receiver: s.wordpress.Tag().String(), Name: "fakeaction"},
			// Missing name.
			{Receiver: s.mysqlUnit.Tag().String(), Parameters: expectedParameters},
			// Good (leader syntax).
			{Receiver: "wordpress/leader", Name: expectedName, Parameters: expectedParameters},
		},
	}

	// blocking changes should have no effect
	s.BlockAllChanges(c, "Enqueue")

	res, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Actions, gc.HasLen, 5)

	emptyActionTag := names.ActionTag{}
	c.Assert(res.Actions[0].Error, gc.DeepEquals,
		&params.Error{Message: fmt.Sprintf("%s not valid", arg.Actions[0].Receiver), Code: ""})
	c.Assert(res.Actions[0].Result, gc.Equals, "")

	c.Assert(res.Actions[1].Error, gc.IsNil)
	c.Assert(res.Actions[1].Result, gc.Not(gc.Equals), emptyActionTag)

	errorString := fmt.Sprintf("action receiver interface on entity %s not implemented", arg.Actions[2].Receiver)
	c.Assert(res.Actions[2].Error, gc.DeepEquals, &params.Error{Message: errorString, Code: "not implemented"})
	c.Assert(res.Actions[2].Result, gc.Equals, "")

	c.Assert(res.Actions[3].Error, gc.ErrorMatches, "no action name given")
	c.Assert(res.Actions[3].Result, gc.Equals, "")

	c.Assert(res.Actions[4].Error, gc.IsNil)
	c.Assert(res.Actions[4].Result, gc.Not(gc.Equals), emptyActionTag)

	// Make sure that 2 actions were enqueued for the wordpress Unit.
	actions, err = s.wordpressUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 2)
	for _, act := range actions {
		c.Assert(act.Name(), gc.Equals, expectedName)
		c.Assert(act.Parameters(), gc.DeepEquals, expectedParameters)
		c.Assert(act.Receiver(), gc.Equals, s.wordpressUnit.Name())
	}

	// Make sure an Action was not enqueued for the mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 0)

	operations, err := s.Model.AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Summary(), gc.Equals, "multiple actions run on unit-wordpress-0,application-wordpress,unit-mysql-0,wordpress/leader")
}

func (s *operationSuite) TestOperationsStatusFilter(c *gc.C) {
	s.setupOperations(c)
	actions, err := s.action.Operations(params.OperationQueryArgs{
		Status: []string{"running"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions.Results, gc.HasLen, 1)
	result := actions.Results[0]
	c.Assert(result.Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result.Status, gc.Equals, "running")
	c.Assert(result.Action.Name, gc.Equals, "fakeaction")
	c.Assert(result.Action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(result.Action.Tag, gc.Equals, "action-2")
}

func (s *operationSuite) TestOperationsNameFilter(c *gc.C) {
	s.setupOperations(c)
	actions, err := s.action.Operations(params.OperationQueryArgs{
		FunctionNames: []string{"anotherfakeaction"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions.Results, gc.HasLen, 1)
	result := actions.Results[0]
	c.Assert(result.Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(result.Status, gc.Equals, "pending")
	c.Assert(result.Action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(result.Action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(result.Action.Tag, gc.Equals, "action-5")
}

func (s *operationSuite) TestOperationsAppFilter(c *gc.C) {
	s.setupOperations(c)
	actions, err := s.action.Operations(params.OperationQueryArgs{
		Applications: []string{"wordpress"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions.Results, gc.HasLen, 2)
	result0 := actions.Results[0]
	result1 := actions.Results[1]

	c.Assert(result0.Action, gc.NotNil)
	if result0.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(result0.Status, gc.Equals, "pending")
	c.Assert(result0.Action.Name, gc.Equals, "fakeaction")
	c.Assert(result0.Action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(result0.Action.Tag, gc.Equals, "action-4")

	c.Assert(result1.Action, gc.NotNil)
	if result1.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result1.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result1.Status, gc.Equals, "running")
	c.Assert(result1.Action.Name, gc.Equals, "fakeaction")
	c.Assert(result1.Action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(result1.Action.Tag, gc.Equals, "action-2")
}

func (s *operationSuite) TestOperationsUnitFilter(c *gc.C) {
	s.setupOperations(c)
	actions, err := s.action.Operations(params.OperationQueryArgs{
		Units:  []string{"wordpress/0"},
		Status: []string{"pending"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions.Results, gc.HasLen, 1)
	result := actions.Results[0]

	c.Assert(result.Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(result.Status, gc.Equals, "pending")
	c.Assert(result.Action.Name, gc.Equals, "fakeaction")
	c.Assert(result.Action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(result.Action.Tag, gc.Equals, "action-4")
}

func (s *operationSuite) TestOperationsAppAndUnitFilter(c *gc.C) {
	s.setupOperations(c)
	actions, err := s.action.Operations(params.OperationQueryArgs{
		Applications: []string{"mysql"},
		Units:        []string{"wordpress/0"},
		Status:       []string{"pending"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions.Results, gc.HasLen, 2)
	mysqlAction := actions.Results[0]
	wordpressAction := actions.Results[1]
	c.Log(pretty.Sprint(actions.Results))

	c.Assert(mysqlAction.Action, gc.NotNil)
	if mysqlAction.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(mysqlAction.Status, gc.Equals, "pending")
	c.Assert(mysqlAction.Action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(mysqlAction.Action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(mysqlAction.Action.Tag, gc.Equals, "action-5")

	c.Assert(wordpressAction.Action, gc.NotNil)
	if wordpressAction.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(wordpressAction.Status, gc.Equals, "pending")
	c.Assert(wordpressAction.Action.Name, gc.Equals, "fakeaction")
	c.Assert(wordpressAction.Action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(wordpressAction.Action.Tag, gc.Equals, "action-4")

}
