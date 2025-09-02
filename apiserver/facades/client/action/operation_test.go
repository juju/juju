// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/rpc/params"
)

type operationSuite struct {
	MockBaseSuite
}

func TestOperationSuite(t *testing.T) {
	// Keep legacy runner but now we populate with real tests
	tc.Run(t, &operationSuite{})
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
		IsParallel:     true, // defaulted to true
		ExecutionGroup: "",   // defaulted to ""
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

// TestListOperations_PermissionDenied verifies ListOperations returns ErrPerm
// and does not call service when read permission is denied.
func (s *operationSuite) TestListOperations_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	// Authorizer without read permission
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService, s.BlockCommandService, s.ModelInfoService, s.OperationService, modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure List is not called
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.ListOperations(c.Context(), params.OperationQueryArgs{Applications: []string{"app"}})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestListOperations_NoFilters verifies that no filters pass an empty target
// and no other filters, and empty result is returned.
func (s *operationSuite) TestListOperations_NoFilters(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Target.Applications, tc.HasLen, 0)
			c.Check(qp.Target.Machines, tc.HasLen, 0)
			c.Check(qp.Target.Units, tc.HasLen, 0)
			c.Check(qp.ActionNames, tc.IsNil)
			c.Check(qp.Status, tc.IsNil)
			c.Check(qp.Limit, tc.IsNil)
			c.Check(qp.Offset, tc.IsNil)
			return operation.QueryResult{}, nil
		})

	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ApplicationsFilter ensures application names flow into
// Target.Applications.
func (s *operationSuite) TestListOperations_ApplicationsFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	apps := []string{"app-a", "app-b"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Target.Applications, tc.DeepEquals, apps)
			return operation.QueryResult{}, nil
		})

	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Applications: apps})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_UnitsFilter verifies string unit names convert to
// []unit.Name in Target.Units.
func (s *operationSuite) TestListOperations_UnitsFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	units := []string{"app-a/0", "app-b/3"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Target.Units, tc.DeepEquals, []unit.Name{"app-a/0", "app-b/3"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Units: units})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_MachinesFilter verifies string machine names convert
// to []machine.Name in Target.Machines.
func (s *operationSuite) TestListOperations_MachinesFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	machines := []string{"0", "42"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Target.Machines, tc.DeepEquals, []machine.Name{"0", "42"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Machines: machines})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ActionNamesFilter confirms actions filter passes through.
func (s *operationSuite) TestListOperations_ActionNamesFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	names := []string{"backup", "reindex"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.ActionNames, tc.DeepEquals, names)
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{ActionNames: names})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_StatusFilter confirms status filter passes through.
func (s *operationSuite) TestListOperations_StatusFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	status := []string{"running", "completed"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Status, tc.DeepEquals, status)
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Status: status})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_LimitOffset verifies Limit and Offset pointers pass unchanged.
func (s *operationSuite) TestListOperations_LimitOffset(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	limit := 10
	offset := 20
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Limit, tc.DeepEquals, ptr(10))
			c.Check(qp.Offset, tc.DeepEquals, ptr(20))
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Limit: &limit, Offset: &offset})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_CombinedFilters ensures multiple filters are passed
// together without modification.
func (s *operationSuite) TestListOperations_CombinedFilters(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	apps := []string{"a"}
	units := []string{"a/0"}
	machines := []string{"1"}
	actionNames := []string{"do"}
	status := []string{"running"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Target.Applications, tc.DeepEquals, apps)
			c.Check(qp.ActionNames, tc.DeepEquals, actionNames)
			c.Check(qp.Status, tc.DeepEquals, status)
			c.Check(qp.Target.Units, tc.DeepEquals, []unit.Name{"a/0"})
			c.Check(qp.Target.Machines, tc.DeepEquals, []machine.Name{"1"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{
		Applications: apps,
		Units:        units,
		Machines:     machines,
		ActionNames:  actionNames,
		Status:       status})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ServiceError ensures service error is propagated.
func (s *operationSuite) TestListOperations_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(operation.QueryResult{}, fmt.Errorf("boom"))
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestListOperations_MappingSingleOperation validates mapping of
// OperationInfo with unit and machine actions into params.
func (s *operationSuite) TestListOperations_MappingSingleOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	tiM := operation.TaskInfo{ID: "1", TaskArgs: operation.TaskArgs{ActionName: "m-act"}}
	tiU := operation.TaskInfo{ID: "2", TaskArgs: operation.TaskArgs{ActionName: "u-act"}}
	qr := operation.QueryResult{Operations: []operation.OperationInfo{{
		OperationID: "123",
		Summary:     "s",
		Status:      "completed",
		Machines:    []operation.MachineTaskResult{{ReceiverName: "2", TaskInfo: tiM}},
		Units:       []operation.UnitTaskResult{{ReceiverName: "app/0", TaskInfo: tiU}},
	}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Results), tc.Equals, 1)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-123")
	c.Check(res.Results[0].Summary, tc.Equals, "s")
	c.Check(res.Results[0].Status, tc.Equals, "completed")
	c.Check(len(res.Results[0].Actions), tc.Equals, 2)
	// machine action
	c.Check(res.Results[0].Actions[0].Action.Receiver, tc.Equals, "machine-2")
	c.Check(res.Results[0].Actions[0].Action.Name, tc.Equals, "m-act")
	c.Check(res.Results[0].Actions[0].Action.Tag, tc.Equals, names.NewActionTag("1").String())
	// unit action
	c.Check(res.Results[0].Actions[1].Action.Receiver, tc.Equals, "unit-app-0")
	c.Check(res.Results[0].Actions[1].Action.Name, tc.Equals, "u-act")
	c.Check(res.Results[0].Actions[1].Action.Tag, tc.Equals, names.NewActionTag("2").String())
}

// TestListOperations_TruncatedPassThrough ensures Truncated flag propagates.
func (s *operationSuite) TestListOperations_TruncatedPassThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Truncated: true}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Truncated, tc.Equals, true)
}

// TestListOperations_OperationErrorMapping validates mapping of operation-level error to params.Error.
func (s *operationSuite) TestListOperations_OperationErrorMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Operations: []operation.OperationInfo{{OperationID: "1", Error: fmt.Errorf("op-fail")}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.NotNil)
	c.Assert(res.Results[0].Error.Message, tc.Matches, ".*op-fail.*")
}

// TestListOperations_ActionFieldMapping ensures TaskInfo fields map into
// ActionResult fields.
func (s *operationSuite) TestListOperations_ActionFieldMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	when := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	log := []operation.TaskLog{{Timestamp: when, Message: "log1"}}
	ti := operation.TaskInfo{
		ID:       "1",
		TaskArgs: operation.TaskArgs{ActionName: "run"},
		Status:   "running",
		Message:  "in progress",
		Log:      log,
		Output:   map[string]interface{}{"k": "v"},
		Error:    fmt.Errorf("task-fail")}
	qr := operation.QueryResult{
		Operations: []operation.OperationInfo{{
			OperationID: "1",
			Units: []operation.UnitTaskResult{{
				ReceiverName: "app/1",
				TaskInfo:     ti,
			}}}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)

	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	acts := res.Results[0].Actions
	c.Assert(acts, tc.HasLen, 1)
	ar := acts[0]
	c.Check(ar.Status, tc.Equals, "running")
	c.Check(ar.Message, tc.Equals, "in progress")
	c.Check(ar.Log, tc.HasLen, 1)
	c.Check(ar.Log[0].Timestamp.Equal(when), tc.Equals, true)
	c.Check(ar.Log[0].Message, tc.Equals, "log1")
	c.Check(ar.Output["k"], tc.Equals, "v")
	c.Assert(ar.Error, tc.NotNil)
	c.Check(ar.Error.Message, tc.Matches, ".*task-fail.*")
}

// TestListOperations_EmptyOperations verifies that an empty operations slice results in empty results.
func (s *operationSuite) TestListOperations_EmptyOperations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Operations: []operation.OperationInfo{}, Truncated: false}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Results), tc.Equals, 0)
	c.Assert(res.Truncated, tc.Equals, false)
}

// toEntities converts tags to params.Entities for Operations tests.
func toEntities(tags ...string) params.Entities {
	ents := make([]params.Entity, len(tags))
	for i, t := range tags {
		ents[i] = params.Entity{Tag: t}
	}
	return params.Entities{Entities: ents}
}

// TestOperations_PermissionDenied verifies read permission is enforced
// and that the service is not called when denied.
func (s *operationSuite) TestOperations_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService,
		s.BlockCommandService, s.ModelInfoService, s.OperationService,
		modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure no call
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.Operations(c.Context(), toEntities("operation-1"))

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestOperations_AllTagsInvalid returns per-entity parse errors and
// does not call the service.
func (s *operationSuite) TestOperations_AllTagsInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	// No service call expected
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), gomock.Any()).Times(0)
	arg := toEntities("not-a-tag", "application-foo", "unit-app-0")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 3)
	for i := range res.Results {
		c.Check(res.Results[i].Error, tc.NotNil)
	}
}

// TestOperations_MixedValidInvalid calls service with only valid IDs and
// aligns results in input order with parse errors preserved.
func (s *operationSuite) TestOperations_MixedValidInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1", "2"}).DoAndReturn(
		func(ctx context.Context, ids []string) (operation.QueryResult, error) {
			return operation.QueryResult{Operations: []operation.OperationInfo{{
				OperationID: "1",
				Summary:     "a",
			}, {
				OperationID: "2",
				Summary:     "b",
			}}}, nil
		})
	arg := toEntities("operation-1", "bad-tag", "operation-2")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 3)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-1")
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Summary, tc.Equals, "a")
	c.Check(res.Results[1].Error, tc.NotNil)
	c.Check(res.Results[2].OperationTag, tc.Equals, "operation-2")
	c.Check(res.Results[2].Error, tc.IsNil)
	c.Check(res.Results[2].Summary, tc.Equals, "b")
}

// TestOperations_EmptyInput returns empty results and does not call service.
func (s *operationSuite) TestOperations_EmptyInput(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), gomock.Any()).Times(0)

	// Act
	res, err := api.Operations(c.Context(), params.Entities{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 0)
}

// TestOperations_ServiceError ensures service errors are propagated.
func (s *operationSuite) TestOperations_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1", "2"}).Return(
		operation.QueryResult{}, fmt.Errorf("boom"))
	arg := toEntities("operation-1", "operation-2")

	// Act
	_, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestOperations_MappingOutOfOrder maps out-of-order service results to the
// correct input positions by tag.
func (s *operationSuite) TestOperations_MappingOutOfOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1", "2", "3"}).Return(
		operation.QueryResult{Operations: []operation.OperationInfo{{
			OperationID: "3",
		}, {
			OperationID: "1",
		}, {
			OperationID: "2",
		}}}, nil)
	arg := toEntities("operation-1", "operation-2", "operation-3")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-1")
	c.Check(res.Results[1].OperationTag, tc.Equals, "operation-2")
	c.Check(res.Results[2].OperationTag, tc.Equals, "operation-3")
}

// TestOperations_UnexpectedTag errors when the service returns an operation
// not requested, by tag.
func (s *operationSuite) TestOperations_UnexpectedTag(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1"}).Return(
		operation.QueryResult{Operations: []operation.OperationInfo{{
			OperationID: "999",
		}}}, nil)
	arg := toEntities("operation-1")

	// Act
	_, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "unexpected result for \"operation-999\"")
}

// TestOperations_DuplicateTags validates the behavior of the operation API
// when duplicate tags are provided in the request.
func (s *operationSuite) TestOperations_DuplicateTags(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1", "1"}).Return(
		operation.QueryResult{Operations: []operation.OperationInfo{{
			OperationID: "1", // Only one operation to domain
		}}}, nil)
	arg := toEntities("operation-1", "operation-1")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert: operation is duplicated in result
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[1].OperationTag, tc.Equals, "operation-1")
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-1")
	c.Check(res.Results[0].Error, tc.IsNil)
}

// TestOperations_MissingServiceReturn allows missing returns without error.
func (s *operationSuite) TestOperations_MissingServiceReturn(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), []string{"1", "2"}).Return(
		operation.QueryResult{Operations: []operation.OperationInfo{{
			OperationID: "2",
		}}}, nil)
	arg := toEntities("bad", "operation-1", "operation-2", "also-bad")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].Error, tc.NotNil) // bad tag
	c.Check(res.Results[1].OperationTag, tc.Equals, "")
	c.Check(res.Results[1].Error, tc.ErrorMatches, ".*not found.*") // no results
	c.Check(res.Results[2].OperationTag, tc.Equals, "operation-2")
	c.Check(res.Results[3].Error, tc.NotNil) // bad tag
}

// TestOperations_LargeBatch ensures stable mapping for many entries.
func (s *operationSuite) TestOperations_LargeBatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	ids := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	s.OperationService.EXPECT().GetOperationsByIDs(gomock.Any(), ids).Return(
		operation.QueryResult{
			Operations: []operation.OperationInfo{
				{OperationID: "8"},
				{OperationID: "7"},
				{OperationID: "6"},
				{OperationID: "5"},
				{OperationID: "4"},
				{OperationID: "3"},
				{OperationID: "2"},
				{OperationID: "1"},
			},
		}, nil)
	arg := toEntities(
		"operation-1", "operation-2", "operation-3", "operation-4",
		"operation-5", "operation-6", "operation-7", "operation-8",
	)

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	for i := 1; i <= 8; i++ {
		c.Check(res.Results[i-1].OperationTag, tc.Equals, fmt.Sprintf("operation-%d", i))
	}
}

func ptr[T any](v T) *T { return &v }
