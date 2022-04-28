// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"strconv"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/action"
	"github.com/juju/juju/apiserver/facades/client/action/mocks"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type operationSuite struct {
	baseSuite
}

var _ = gc.Suite(&operationSuite{})

func (s *operationSuite) setupOperations(c *gc.C) {
	parallel := true
	executionGroup := "group"
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{},
				Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "anotherfakeaction", Parameters: map[string]interface{}{}},
		}}

	r, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Actions, gc.HasLen, len(arg.Actions))

	// There's only one operation created.
	ops, err := s.Model.AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)
	operationID, err := strconv.Atoi(ops[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	a, err := s.Model.Action(strconv.Itoa(operationID + 1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.Parallel(), jc.IsTrue)
	c.Assert(a.ExecutionGroup(), gc.Equals, "group")
	_, err = a.Begin()
	c.Assert(err, jc.ErrorIsNil)
	a, err = s.Model.Action(strconv.Itoa(operationID + 2))
	c.Assert(err, jc.ErrorIsNil)
	_, err = a.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *operationSuite) TestListOperationsStatusFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up a non running operation.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Status: []string{"running"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Truncated, jc.IsFalse)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]
	c.Assert(result.Actions, gc.HasLen, 4)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result.Status, gc.Equals, "running")

	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-2")
	c.Assert(result.Actions[0].Status, gc.Equals, "running")
	action = result.Actions[1].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-3")
	c.Assert(result.Actions[1].Status, gc.Equals, "completed")
	action = result.Actions[2].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-4")
	c.Assert(result.Actions[2].Status, gc.Equals, "pending")
	action = result.Actions[3].Action
	c.Assert(action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-5")
	c.Assert(result.Actions[3].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestListOperationsNameFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up a second operation.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		ActionNames: []string{"anotherfakeaction"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]
	c.Assert(result.Actions, gc.HasLen, 1)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result.Status, gc.Equals, "running")
	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-5")
	c.Assert(result.Actions[0].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestListOperationsAppFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up a second operation for a different app.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Applications: []string{"wordpress"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]

	c.Assert(result.Actions, gc.HasLen, 2)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result.Status, gc.Equals, "running")
	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-2")
	c.Assert(result.Actions[0].Status, gc.Equals, "running")
	action = result.Actions[1].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-4")
	c.Assert(result.Actions[1].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestListOperationsUnitFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up an operation with a pending action.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Units:  []string{"wordpress/0"},
		Status: []string{"pending"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]

	c.Assert(result.Actions, gc.HasLen, 1)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(result.Status, gc.Equals, "pending")
	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-7")
	c.Assert(result.Actions[0].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestListOperationsMachineFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up an operation with a pending action.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.machine0.Tag().String(), Name: "juju-exec", Parameters: map[string]interface{}{
				"command": "ls",
				"timeout": 1,
			}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Machines: []string{"0"},
		Status:   []string{"pending"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]

	c.Assert(result.Actions, gc.HasLen, 1)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	c.Assert(result.Status, gc.Equals, "pending")
	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "juju-exec")
	c.Assert(action.Receiver, gc.Equals, "machine-0")
	c.Assert(action.Tag, gc.Equals, "action-7")
	c.Assert(result.Actions[0].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestListOperationsAppAndUnitFilter(c *gc.C) {
	s.setupOperations(c)
	// Set up an operation with a pending action.
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
		}}
	_, err := s.action.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Applications: []string{"mysql"},
		Units:        []string{"wordpress/0"},
		Status:       []string{"running"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Results, gc.HasLen, 1)
	c.Log(pretty.Sprint(operations.Results))
	result := operations.Results[0]

	c.Assert(result.Actions, gc.HasLen, 4)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}

	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-2")
	c.Assert(result.Actions[0].Status, gc.Equals, "running")
	action = result.Actions[1].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-3")
	c.Assert(result.Actions[1].Status, gc.Equals, "completed")
	action = result.Actions[2].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-4")
	c.Assert(result.Actions[2].Status, gc.Equals, "pending")
	action = result.Actions[3].Action
	c.Assert(action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-5")
	c.Assert(result.Actions[3].Status, gc.Equals, "pending")
}

func (s *operationSuite) TestOperations(c *gc.C) {
	s.setupOperations(c)
	operations, err := s.action.Operations(params.Entities{
		Entities: []params.Entity{{Tag: "operation-1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations.Truncated, jc.IsFalse)
	c.Assert(operations.Results, gc.HasLen, 1)
	result := operations.Results[0]
	c.Assert(result.Actions, gc.HasLen, 4)
	c.Assert(result.Actions[0].Action, gc.NotNil)
	if result.Enqueued.IsZero() {
		c.Fatal("enqueued time not set")
	}
	if result.Started.IsZero() {
		c.Fatal("started time not set")
	}
	c.Assert(result.Status, gc.Equals, "running")

	action := result.Actions[0].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-2")
	c.Assert(result.Actions[0].Status, gc.Equals, "running")
	action = result.Actions[1].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-3")
	c.Assert(result.Actions[1].Status, gc.Equals, "completed")
	action = result.Actions[2].Action
	c.Assert(action.Name, gc.Equals, "fakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-wordpress-0")
	c.Assert(action.Tag, gc.Equals, "action-4")
	c.Assert(result.Actions[2].Status, gc.Equals, "pending")
	action = result.Actions[3].Action
	c.Assert(action.Name, gc.Equals, "anotherfakeaction")
	c.Assert(action.Receiver, gc.Equals, "unit-mysql-0")
	c.Assert(action.Tag, gc.Equals, "action-5")
	c.Assert(result.Actions[3].Status, gc.Equals, "pending")
}

type enqueueSuite struct {
	wordpressAction *mocks.MockAction
	mysqlAction     *mocks.MockAction
	actionReceiver  *mocks.MockActionReceiver
	authorizer      *facademocks.MockAuthorizer
	model           *mocks.MockModel
	state           *mocks.MockState

	modelTag         names.ModelTag
	wordpressUnitTag names.UnitTag
	mysqlUnitTag     names.UnitTag
}

var _ = gc.Suite(&enqueueSuite{})

func (s *enqueueSuite) SetUpSuite(c *gc.C) {
	s.modelTag = names.NewModelTag("model-tag")
	s.wordpressUnitTag = names.NewUnitTag("wordpress/0")
	s.mysqlUnitTag = names.NewUnitTag("mysql/0")
}

func (s *enqueueSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.authorizer.EXPECT().AuthClient().Return(true)

	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().ModelTag().Return(s.modelTag)

	s.state = mocks.NewMockState(ctrl)
	s.state.EXPECT().Model().Return(s.model, nil)

	s.actionReceiver = mocks.NewMockActionReceiver(ctrl)

	s.wordpressAction = mocks.NewMockAction(ctrl)
	s.mysqlAction = mocks.NewMockAction(ctrl)

	return ctrl
}

func (s *enqueueSuite) expectWordpressActionResult() {
	s.model.EXPECT().AddAction(gomock.Any(), "1", "fakeaction", gomock.Any(), gomock.Any(), gomock.Any()).Return(s.wordpressAction, nil)
	s.actionReceiver.EXPECT().Tag().Return(s.wordpressUnitTag)
	aExp := s.wordpressAction.EXPECT()
	aExp.ActionTag().Return(names.NewActionTag("2"))
	aExp.Status().Return(state.ActionRunning)
	aExp.Name().Return("fakeaction")
	aExp.Parameters().Return(map[string]interface{}{})
	aExp.Messages().Return(nil)
	aExp.Results().Return(map[string]interface{}{}, "result")
	aExp.Started().Return(time.Now())
	aExp.Completed().Return(time.Now())
	aExp.Enqueued().Return(time.Now())
}

func (s *enqueueSuite) expectMysqlActionResult() {
	s.model.EXPECT().AddAction(gomock.Any(), "1", "fakeaction", map[string]interface{}{}, gomock.Any(), gomock.Any()).Return(s.mysqlAction, nil)
	s.actionReceiver.EXPECT().Tag().Return(s.mysqlUnitTag)
	aExp := s.mysqlAction.EXPECT()
	aExp.ActionTag().Return(names.NewActionTag("3"))
	aExp.Status().Return(state.ActionRunning)
	aExp.Name().Return("fakeaction")
	aExp.Parameters().Return(map[string]interface{}{})
	aExp.Messages().Return(nil)
	aExp.Results().Return(map[string]interface{}{}, "result")
	aExp.Started().Return(time.Now())
	aExp.Completed().Return(time.Now())
	aExp.Enqueued().Return(time.Now())
}

func (s *enqueueSuite) getAPI(c *gc.C) *action.ActionAPI {
	api, err := action.NewActionAPIForMockTest(s.state, nil, s.authorizer, s.tagToActionReceiverFn)
	c.Assert(err, gc.IsNil)
	return api
}

func (s *enqueueSuite) tagToActionReceiverFn(_ func(names.Tag) (state.Entity, error)) func(tag string) (state.ActionReceiver, error) {
	return func(tag string) (state.ActionReceiver, error) {
		return s.actionReceiver, nil
	}
}
