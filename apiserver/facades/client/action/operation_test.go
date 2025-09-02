// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/rpc/params"
)

type operationSuite struct {
	MockBaseSuite
	client *ActionAPI
}

func TestOperationSuite(t *testing.T) {
	// Keep legacy runner but now we populate with real tests
	tc.Run(t, &operationSuite{})
}

func (s *operationSuite) TestStub(c *tc.C) {
	defer s.setupMocks(c).Finish()
	c.Skip(`This suite is missing tests for the following scenarios:
- ListOperations querying by status.
- ListOperations querying by action names.
- ListOperations querying by application names.
- ListOperations querying by unit names.
- ListOperations querying by machines.
- ListOperations querying with multiple filters - result is union.
- Operations based on input entity tags.
- EnqueueOperation with some units
- EnqueueOperation but AddAction fails
- EnqueueOperation with a unit specified with a leader receiver
`)
}

// TestEnqueue_PermissionDenied verifies that enqueuing an operation without proper permission returns ErrPerm.
func (s *operationSuite) TestEnqueue_PermissionDenied(c *tc.C) {

	defer s.setupMocks(c).Finish()
	// Arrange : FakeAuthorizer without write permission should yield ErrPerm
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService, s.BlockCommandService, s.ModelInfoService,
		s.OperationService, modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)

	// Act
	_, err = api.EnqueueOperation(context.Background(), params.Actions{Actions: []params.Action{{Receiver: "app/0", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestEnqueue_NoActions verifies that enqueuing an operation with no actions results
// in an appropriate error response.
func (s *operationSuite) TestEnqueue_NoActions(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{})

	// Assert
	c.Assert(err, tc.ErrorMatches, "no actions specified")
}

// TestEnqueue_SingleUnit verifies the enqueue operation for a single unit with
// actions, parameters, and execution groups.
func (s *operationSuite) TestEnqueue_SingleUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do",
		Parameters:     map[string]interface{}{"k": "v"},
		IsParallel:     true,
		ExecutionGroup: "grp",
	}
	s.OperationService.EXPECT().Run(gomock.Any(), []operation.RunArgs{{
		Target: operation.Target{
			Units: []unit.Name{"app/0"},
		},
		TaskArgs: taskArgs,
	},
	}).Return(operation.RunResult{
		OperationID: "1",
		Units: []operation.UnitTaskResult{{
			ReceiverName: "app/0",
			TaskInfo: operation.TaskInfo{
				ID:       "1",
				TaskArgs: taskArgs,
			}}}}, nil)

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{
		Receiver:       "unit-app-0",
		Name:           "do",
		Parameters:     map[string]interface{}{"k": "v"},
		Parallel:       ptr(true),
		ExecutionGroup: ptr("grp")}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-1")
	c.Assert(res.Actions, tc.HasLen, 1)
	c.Check(res.Actions[0].Error, tc.IsNil)
	c.Check(res.Actions[0].Action, tc.DeepEquals, &params.Action{
		Tag:            "action-1",
		Receiver:       "unit-app-0",
		Name:           "do",
		Parameters:     map[string]interface{}{"k": "v"},
		Parallel:       ptr(true),
		ExecutionGroup: ptr("grp"),
	})
}

// TestEnqueue_LeaderReceiver verifies the enqueue operation behavior when
// the receiver is the leader unit of an application.
func (s *operationSuite) TestEnqueue_LeaderReceiver(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName: "do",
	}
	s.OperationService.EXPECT().Run(gomock.Any(), []operation.RunArgs{{
		Target: operation.Target{
			LeaderUnit: []string{"myapp"},
		},
		TaskArgs: taskArgs,
	},
	}).Return(operation.RunResult{
		OperationID: "2",
		Units: []operation.UnitTaskResult{{
			ReceiverName: "myapp/0",
			IsLeader:     true,
			TaskInfo: operation.TaskInfo{
				ID:       "1",
				TaskArgs: taskArgs,
			}}}}, nil)

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "myapp/leader",
		Name: "do", Parallel: ptr(false)}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-2")
	c.Assert(len(res.Actions), tc.Equals, 1)
	c.Assert(res.Actions[0].Error, tc.IsNil)
	c.Assert(res.Actions[0].Action, tc.NotNil)
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "unit-myapp-0")
}

// TestEnqueue_Defaults verifies the enqueue operation applies default values
// for isParallel and ExecutionGroup parameters.
func (s *operationSuite) TestEnqueue_Defaults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do-default",
		IsParallel:     false,
		ExecutionGroup: "", // defaulted to ""
	}
	s.OperationService.EXPECT().Run(gomock.Any(), []operation.RunArgs{{
		Target: operation.Target{
			Units: []unit.Name{"app/0"},
		},
		TaskArgs: taskArgs,
	}}).Return(operation.RunResult{OperationID: "404" /*placeholder, we check the input args */}, nil)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{
		Receiver: "unit-app-0",
		Name:     "do-default",
		// default values for isParallel and Execution group
	}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnqueue_MultipleActions validates the enqueue operation for multiple
// actions with correct execution order and parameters.
func (s *operationSuite) TestEnqueue_MultipleActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			c.Assert(len(got), tc.Equals, 3)
			// order: leader, app/2, app/0
			c.Assert(got[0].LeaderUnit, tc.DeepEquals, []string{"app"})
			c.Assert(got[1].Units, tc.DeepEquals, []unit.Name{"app/2"})
			c.Assert(got[2].Units, tc.DeepEquals, []unit.Name{"app/0"})
			ti1 := operation.TaskInfo{ID: "1", TaskArgs: got[0].TaskArgs}
			ti2 := operation.TaskInfo{ID: "2", TaskArgs: got[1].TaskArgs}
			ti3 := operation.TaskInfo{ID: "3", TaskArgs: got[2].TaskArgs}
			return operation.RunResult{OperationID: "3",
				Units: []operation.UnitTaskResult{
					{ReceiverName: "app/0", TaskInfo: ti3},
					{ReceiverName: "app/2", TaskInfo: ti2},
					{ReceiverName: "app/9", IsLeader: true, TaskInfo: ti1},
				},
			}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "app/leader",
		Name: "x"}, {Receiver: "unit-app-2", Name: "y"}, {Receiver: "unit-app-0", Name: "z"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Actions), tc.Equals, 3)
	c.Check(res.Actions[0].Action.Name, tc.Equals, "x")
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "unit-app-9")
	c.Check(res.Actions[1].Action.Name, tc.Equals, "y")
	c.Check(res.Actions[1].Action.Receiver, tc.Equals, "unit-app-2")
	c.Check(res.Actions[2].Action.Name, tc.Equals, "z")
	c.Check(res.Actions[2].Action.Receiver, tc.Equals, "unit-app-0")
}

// TestEnqueue_SomeInvalid validates the behavior of the EnqueueOperation method
// when some provided actions are invalid (receiver with a bad tag)
func (s *operationSuite) TestEnqueue_SomeInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			c.Assert(len(got), tc.Equals, 1)
			ti := operation.TaskInfo{ID: "1", TaskArgs: got[0].TaskArgs}
			return operation.RunResult{OperationID: "4", Units: []operation.UnitTaskResult{{ReceiverName: unit.Name(
				"app/3"), TaskInfo: ti}}}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "badformat/0",
		Name: "do"}, {Receiver: "unit-app-3", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-4")
	c.Assert(res.Actions[0].Error, tc.NotNil)
	c.Assert(res.Actions[1].Error, tc.IsNil)
}

// TestEnqueue_AllInvalid_NoServiceCall verifies that no service call is made
// when all actions have invalid receivers.
func (s *operationSuite) TestEnqueue_AllInvalid_NoServiceCall(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	// Ensure Run is not called
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

	// Act: all tag receiver are invalid
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "bad1", Name: "do"}, {Receiver: "also/bad", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "")
	c.Assert(res.Actions[0].Error, tc.NotNil)
	c.Assert(res.Actions[1].Error, tc.NotNil)
}

// TestEnqueue_ServiceError checks that EnqueueOperation returns an error when the OperationService.Run fails.
func (s *operationSuite) TestEnqueue_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Return(operation.RunResult{}, fmt.Errorf("boom"))
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestEnqueue_UnexpectedExtraResult verifies the behavior when an unexpected
// extra result is returned during operation execution.
func (s *operationSuite) TestEnqueue_UnexpectedExtraResult(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			return operation.RunResult{
				OperationID: "5",
				Units: []operation.UnitTaskResult{
					{
						ReceiverName: "otherapp/9", // this result is not expected
						TaskInfo:     operation.TaskInfo{ID: "0"},
					}}}, nil
		})
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "unexpected result for \"otherapp/9\"")
}

// TestEnqueue_MissingResultPerActionError verifies that EnqueueOperation
// returns an error when results are missing for actions.
func (s *operationSuite) TestEnqueue_MissingResultPerActionError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.NewActionAPI(c)
	// Arrange
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			// only return app/0 result; missing app/1
			ti := operation.TaskInfo{ID: "0", TaskArgs: got[0].TaskArgs}
			return operation.RunResult{OperationID: "8", Units: []operation.UnitTaskResult{{ReceiverName: unit.Name(
				"app/0"), TaskInfo: ti}}}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}, {Receiver: "unit-app-1", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-8")
	c.Assert(res.Actions[0].Error, tc.IsNil)
	c.Assert(res.Actions[1].Error, tc.NotNil)
}

func ptr[T any](v T) *T { return &v }
