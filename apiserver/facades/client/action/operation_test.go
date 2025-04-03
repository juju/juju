// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
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

func (s *operationSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- ListOperations querying by status.
- ListOperations querying by action names.
- ListOperations querying by application names.
- ListOperations querying by unit names.
- ListOperations querying by machines.
- ListOperations querying with multiple filters - result is union.
- Operations based on input entity tags.
`)
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

	r, err := api.EnqueueOperation(context.Background(), arg)
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

	r, err := api.EnqueueOperation(context.Background(), arg)
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

	r, err := api.EnqueueOperation(context.Background(), arg)
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
	s.Authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.Authorizer.EXPECT().AuthClient().Return(true)

	s.State = action.NewMockState(ctrl)
	s.model = action.NewMockModel(ctrl)
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
