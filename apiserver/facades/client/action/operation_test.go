// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/action"
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
	ops, err := s.ControllerModel(c).AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)
	operationID, err := strconv.Atoi(ops[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	a, err := s.ControllerModel(c).Action(strconv.Itoa(operationID + 1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.Parallel(), jc.IsTrue)
	c.Assert(a.ExecutionGroup(), gc.Equals, "group")
	_, err = a.Begin()
	c.Assert(err, jc.ErrorIsNil)
	a, err = s.ControllerModel(c).Action(strconv.Itoa(operationID + 2))
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
	action.MockBaseSuite

	wordpressAction *action.MockAction
	mysqlAction     *action.MockAction
	model           *action.MockModel

	modelTag         names.ModelTag
	wordpressUnitTag names.UnitTag
	mysqlUnitTag     names.UnitTag
	executionGroup   string
}

var _ = gc.Suite(&enqueueSuite{})

func (s *enqueueSuite) SetUpSuite(c *gc.C) {
	s.modelTag = names.NewModelTag("model-tag")
	// mysql will be parallel false
	s.wordpressUnitTag = names.NewUnitTag("wordpress/0")
	// mysql will be parallel true
	s.mysqlUnitTag = names.NewUnitTag("mysql/0")
	s.executionGroup = "testgroup"
}

func (s *enqueueSuite) TestEnqueueOperation(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.model.EXPECT().EnqueueOperation(gomock.Any(), 2).Return("1", nil)
	s.expectWordpressActionResult()
	s.expectMysqlActionResult()

	api := s.NewActionAPI(c)

	expectedName := "fakeaction"
	f := false
	t := true
	arg := params.Actions{
		Actions: []params.Action{
			{
				Receiver:       s.wordpressUnitTag.String(),
				Name:           expectedName,
				Parameters:     map[string]interface{}{},
				Parallel:       &f,
				ExecutionGroup: &s.executionGroup,
			}, {
				Receiver:       s.mysqlUnitTag.String(),
				Name:           expectedName,
				Parameters:     map[string]interface{}{},
				Parallel:       &t,
				ExecutionGroup: &s.executionGroup,
			},
		}}

	r, err := api.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Actions, gc.HasLen, len(arg.Actions))
	c.Assert(r.Actions[0].Status, gc.Equals, "running")
	c.Assert(r.Actions[0].Action.Name, gc.Equals, expectedName)
	c.Assert(r.Actions[0].Action.Tag, gc.Equals, "action-2")
	c.Assert(r.Actions[1].Status, gc.Equals, "running")
	c.Assert(r.Actions[1].Action.Name, gc.Equals, expectedName)
	c.Assert(r.Actions[1].Action.Tag, gc.Equals, "action-3")
}

func (s *enqueueSuite) TestEnqueueOperationFail(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedName := "fakeaction"
	s.model.EXPECT().EnqueueOperation(gomock.Any(), 3).Return("1", nil)
	s.expectWordpressActionResult()
	s.model.EXPECT().AddAction(gomock.Any(), "1", expectedName, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.NotFoundf("database txn failure"))
	leaders := map[string]string{
		"test": "test/1",
	}
	s.Leadership.EXPECT().Leaders().Return(leaders, nil)
	s.model.EXPECT().FailOperationEnqueuing("1", "error(s) enqueueing action(s): database txn failure not found, could not determine leader for \"mysql\"", 1)

	api := s.NewActionAPI(c)

	f := false
	arg := params.Actions{
		Actions: []params.Action{
			{
				Receiver:       s.wordpressUnitTag.String(),
				Name:           expectedName,
				Parameters:     map[string]interface{}{},
				Parallel:       &f,
				ExecutionGroup: &s.executionGroup,
			},
			// AddAction failure.
			{Receiver: s.mysqlUnitTag.String(), Name: expectedName, Parameters: map[string]interface{}{}},
			// Leader failure
			{Receiver: "mysql/leader", Name: expectedName, Parameters: map[string]interface{}{}},
		}}

	r, err := api.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Actions, gc.HasLen, len(arg.Actions))
	c.Logf("%s", pretty.Sprint(r.Actions))
	c.Assert(r.Actions[0].Status, gc.Equals, "running")
	c.Assert(r.Actions[0].Action.Name, gc.Equals, expectedName)
	c.Assert(r.Actions[0].Action.Tag, gc.Equals, "action-2")
	c.Assert(r.Actions[1].Error, jc.Satisfies, params.IsCodeNotFoundOrCodeUnauthorized)
	c.Assert(r.Actions[2].Error, gc.DeepEquals, &params.Error{Message: "could not determine leader for \"mysql\"", Code: ""})
}

func (s *enqueueSuite) TestEnqueueOperationLeadership(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.model.EXPECT().EnqueueOperation(gomock.Any(), 2).Return("1", nil)
	appName, _ := names.UnitApplication(s.mysqlUnitTag.Id())
	leaders := map[string]string{
		"test":  "test/1",
		appName: s.mysqlUnitTag.Id(),
	}
	s.Leadership.EXPECT().Leaders().Return(leaders, nil)
	s.expectWordpressActionResult()
	s.expectMysqlActionResult()

	api := s.NewActionAPI(c)

	expectedName := "fakeaction"
	f := false
	t := true
	arg := params.Actions{
		Actions: []params.Action{
			{
				Receiver:       s.wordpressUnitTag.String(),
				Name:           expectedName,
				Parameters:     map[string]interface{}{},
				Parallel:       &f,
				ExecutionGroup: &s.executionGroup,
			}, {
				Receiver:       "mysql/leader",
				Name:           expectedName,
				Parameters:     map[string]interface{}{},
				Parallel:       &t,
				ExecutionGroup: &s.executionGroup,
			},
		}}

	r, err := api.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Actions, gc.HasLen, len(arg.Actions))
	c.Assert(r.Actions[0].Status, gc.Equals, "running")
	c.Assert(r.Actions[0].Action.Name, gc.Equals, expectedName)
	c.Assert(r.Actions[0].Action.Tag, gc.Equals, "action-2")
	c.Assert(r.Actions[1].Status, gc.Equals, "running")
	c.Assert(r.Actions[1].Action.Name, gc.Equals, expectedName)
	c.Assert(r.Actions[1].Action.Tag, gc.Equals, "action-3")
}

func (s *enqueueSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.Authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.Authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.Authorizer.EXPECT().AuthClient().Return(true)

	s.model = action.NewMockModel(ctrl)
	s.model.EXPECT().ModelTag().Return(s.modelTag).MinTimes(1)

	s.State = action.NewMockState(ctrl)
	s.State.EXPECT().Model().Return(s.model, nil)

	s.ActionReceiver = action.NewMockActionReceiver(ctrl)
	s.Leadership = action.NewMockReader(ctrl)

	s.wordpressAction = action.NewMockAction(ctrl)
	s.mysqlAction = action.NewMockAction(ctrl)

	return ctrl
}

func (s *enqueueSuite) expectWordpressActionResult() {
	f := false
	s.model.EXPECT().AddAction(gomock.Any(), "1", "fakeaction", map[string]interface{}{}, &f, &s.executionGroup).Return(s.wordpressAction, nil)
	s.ActionReceiver.EXPECT().Tag().Return(s.wordpressUnitTag)
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
	aExp.Parallel().Return(f)
	aExp.ExecutionGroup().Return(s.executionGroup)
}

func (s *enqueueSuite) expectMysqlActionResult() {
	t := true
	s.model.EXPECT().AddAction(gomock.Any(), "1", "fakeaction", map[string]interface{}{}, &t, &s.executionGroup).Return(s.mysqlAction, nil)
	s.ActionReceiver.EXPECT().Tag().Return(s.mysqlUnitTag)
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
	aExp.Parallel().Return(t)
	aExp.ExecutionGroup().Return(s.executionGroup)
}
