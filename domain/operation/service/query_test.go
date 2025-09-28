// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	operation "github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// querySuite provides tests for query-related service methods.
//
// It embeds serviceSuite to reuse its setup helpers and mocks.
type querySuite struct {
	serviceSuite
}

func TestQuerySuite(t *testing.T) {
	tc.Run(t, &querySuite{})
}

// TestGetMachineTaskIDsWithStatusHappyPath verifies that a valid machine name
// and status filter result in delegating to state and returning the IDs.
func (s *querySuite) TestGetMachineTaskIDsWithStatusHappyPath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("0")
	status := corestatus.Running
	expected := []string{"t-1", "t-2"}
	s.state.EXPECT().GetMachineTaskIDsWithStatus(gomock.Any(), mName.String(), status.String()).Return(expected, nil)

	// Act
	ids, err := s.service(c).GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(ids, tc.DeepEquals, expected)
}

// TestGetMachineTaskIDsWithStatusNameValidationError ensures that an invalid machine
// name triggers a validation error before any state interaction.
func (s *querySuite) TestGetMachineTaskIDsWithStatusNameValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Invalid machine name (empty) should fail validation via coremachine.Name.Validate.
	var mName coremachine.Name

	// No expectation set on state: should not be called on validation error.

	// Act
	_, err := s.service(c).GetMachineTaskIDsWithStatus(c.Context(), mName, corestatus.Running)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetMachineTaskIDsWithStatusStatusValidationError ensures that an invalid
// status triggers a validation error before any state interaction.
func (s *querySuite) TestGetMachineTaskIDsWithStatusStatusValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("0")
	status := corestatus.Allocating // invalid status

	// No expectation set on state: should not be called on validation error.

	// Act
	_, err := s.service(c).GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetMachineTaskIDsWithStatusStateError validates that state errors are
// captured and returned by the service method.
func (s *querySuite) TestGetMachineTaskIDsWithStatusStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("1")
	status := corestatus.Running
	stateErr := errors.New("boom")
	s.state.EXPECT().GetMachineTaskIDsWithStatus(gomock.Any(), mName.String(), status.String()).Return(nil, stateErr)

	// Act
	_, err := s.service(c).GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *querySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	return ctrl
}

func (s *querySuite) service(c *tc.C) *Service {
	// LeadershipService not needed for these tests.
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter, nil)
}

func (s *querySuite) TestGetOperationByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	operationID := "op-123"
	unitTask := operation.UnitTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "task-1",
			Status: corestatus.Completed,
		},
	}
	machineTask := operation.MachineTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "task-2",
			Status: corestatus.Running,
		},
	}
	expected := operation.OperationInfo{
		OperationID: operationID,
		Units:       []operation.UnitTaskResult{unitTask},
		Machines:    []operation.MachineTaskResult{machineTask},
	}
	// The status should be Running (higher priority than Completed)
	s.state.EXPECT().GetOperationByID(gomock.Any(), operationID).Return(expected, nil)

	got, err := s.service(c).GetOperationByID(c.Context(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(got.OperationID, tc.Equals, operationID)
	c.Check(got.Status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestGetOperationByIDCompleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	operationID := "op-123"
	unitTask := operation.UnitTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "task-1",
			Status: corestatus.Completed,
		},
	}
	machineTask := operation.MachineTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "task-2",
			Status: corestatus.Completed,
		},
	}
	expected := operation.OperationInfo{
		OperationID: operationID,
		Units:       []operation.UnitTaskResult{unitTask},
		Machines:    []operation.MachineTaskResult{machineTask},
	}
	// The status should be Running (higher priority than Completed)
	s.state.EXPECT().GetOperationByID(gomock.Any(), operationID).Return(expected, nil)

	got, err := s.service(c).GetOperationByID(c.Context(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(got.OperationID, tc.Equals, operationID)
	c.Check(got.Status, tc.Equals, corestatus.Completed)
}

func (s *querySuite) TestGetOperationByIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedErr := errors.New("not found")
	s.state.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Return(operation.OperationInfo{}, expectedErr)

	_, err := s.service(c).GetOperationByID(c.Context(), "unknown")
	c.Assert(err, tc.ErrorMatches, "not found")
}

func (s *querySuite) TestGetOperationByIDUnknownStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	operationID := "op-123"
	unitTask := operation.UnitTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "task-1",
			Status: corestatus.Unknown,
		},
	}

	expected := operation.OperationInfo{
		OperationID: operationID,
		Units:       []operation.UnitTaskResult{unitTask},
	}
	// The status should be Running (higher priority than Completed)
	s.state.EXPECT().GetOperationByID(gomock.Any(), operationID).Return(expected, nil)

	got, err := s.service(c).GetOperationByID(c.Context(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(got.OperationID, tc.Equals, operationID)
	// Even though we have only one task and its status is Unknown, we don't
	// break. Pending Status is always set in case of error.
	c.Check(got.Status, tc.Equals, corestatus.Pending)
}

func (s *querySuite) TestOperationStatus(c *tc.C) {
	// Simulate a realistic operation with units and machines in various states
	tasks := []operation.TaskInfo{
		{Status: corestatus.Completed},
		{Status: corestatus.Completed},
		{Status: corestatus.Running},
		{Status: corestatus.Failed},
		{Status: corestatus.Pending},
	}
	status, err := operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	// Should pick Running (highest priority among present)
	c.Assert(status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestOperationStatusEmptyTasks(c *tc.C) {
	status, err := operationStatus(nil)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Error)

	status, err = operationStatus([]operation.TaskInfo{})
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Error)
}

func (s *querySuite) TestOperationStatusSingleActiveStatus(c *tc.C) {
	for _, st := range statusActiveOrder {
		tasks := []operation.TaskInfo{{Status: st}}
		status, err := operationStatus(tasks)
		c.Assert(err, tc.IsNil)
		c.Check(status, tc.Equals, st)
	}
}

func (s *querySuite) TestOperationStatusSingleComletedStatus(c *tc.C) {
	for _, st := range statusCompletedOrder {
		tasks := []operation.TaskInfo{{Status: st}}
		status, err := operationStatus(tasks)
		c.Assert(err, tc.IsNil)
		c.Check(status, tc.Equals, st)
	}
}

func (s *querySuite) TestOperationStatusMixedStatuses(c *tc.C) {
	// Order of priority is:
	// Running > Aborting > Pending > Error > Failed > Cancelled > Completed

	// Start with all statuses present, should return Running.
	tasks := []operation.TaskInfo{
		{Status: corestatus.Running},
		{Status: corestatus.Aborting},
		{Status: corestatus.Pending},
		{Status: corestatus.Error},
		{Status: corestatus.Failed},
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err := operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Running)

	// Remove Running, next highest is Aborting.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Aborting},
		{Status: corestatus.Pending},
		{Status: corestatus.Error},
		{Status: corestatus.Failed},
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Aborting)

	// Remove Aborting, next highest is Pending.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Pending},
		{Status: corestatus.Error},
		{Status: corestatus.Failed},
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Pending)

	// Remove Pending, next highest is Error.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Error},
		{Status: corestatus.Failed},
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Error)

	// Remove Error, next highest is Failed.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Failed},
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Failed)

	// Remove Failed, next highest is Cancelled.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Cancelled},
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Cancelled)

	// Remove Cancelled, next highest is Completed.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Completed},
	}
	status, err = operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Completed)
}

func (s *querySuite) TestOperationStatusAllStatusesPresent(c *tc.C) {
	// If all statuses are present, should return Running (highest priority).
	tasks := []operation.TaskInfo{}
	for _, st := range statusActiveOrder {
		tasks = append(tasks, operation.TaskInfo{Status: st})
	}
	for _, st := range statusCompletedOrder {
		tasks = append(tasks, operation.TaskInfo{Status: st})
	}
	status, err := operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestOperationStatusDuplicateStatuses(c *tc.C) {
	// Multiple tasks with same status, should still return that status.
	tasks := []operation.TaskInfo{
		{Status: corestatus.Failed},
		{Status: corestatus.Failed},
		{Status: corestatus.Failed},
	}
	status, err := operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Failed)
}

func (s *querySuite) TestOperationStatusMixedKnownAndUnknown(c *tc.C) {
	// If at least one known status is present, should pick highest priority
	// known.
	type fakeStatus string
	const unknownStatus fakeStatus = "unknown"
	tasks := []operation.TaskInfo{
		{Status: corestatus.Status(unknownStatus)},
		{Status: corestatus.Running},
		{Status: corestatus.Completed},
	}
	status, err := operationStatus(tasks)
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestOperationStatusOnlyUnknownStatuses(c *tc.C) {
	type fakeStatus string
	const unknownStatus fakeStatus = "unknown"
	tasks := []operation.TaskInfo{
		{Status: corestatus.Status(unknownStatus)},
		{Status: corestatus.Status(unknownStatus)},
	}
	_, err := operationStatus(tasks)
	c.Assert(err, tc.ErrorMatches, "unknown status")
}

func (s *querySuite) TestGetOperations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{
		ActionNames: []string{"test-action"},
		Limit:       ptr(10),
	}

	unitTask := operation.UnitTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "2",
			Status: corestatus.Running,
		},
	}
	machineTask := operation.MachineTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:     "3",
			Status: corestatus.Completed,
		},
	}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "1",
				Summary:     "Test operation",
				Units:       []operation.UnitTaskResult{unitTask},
				Machines:    []operation.MachineTaskResult{machineTask},
			},
		},
		Truncated: false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].OperationID, tc.Equals, "1")
	c.Check(result.Operations[0].Summary, tc.Equals, "Test operation")
	// Status should be Running (higher priority than Completed)
	c.Check(result.Operations[0].Status, tc.Equals, corestatus.Running)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsEmptyResult(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{
		ActionNames: []string{"nonexistent-action"},
	}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{},
		Truncated:  false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{}
	stateErr := errors.New("boom")
	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(operation.QueryResult{}, stateErr)

	// Act
	_, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *querySuite) TestGetOperationsMultipleOperationsWithDifferentStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "1",
				Units: []operation.UnitTaskResult{
					{TaskInfo: operation.TaskInfo{ID: "task-1", Status: corestatus.Running}},
					{TaskInfo: operation.TaskInfo{ID: "task-2", Status: corestatus.Completed}},
				},
			},
			{
				OperationID: "2",
				Machines: []operation.MachineTaskResult{
					{TaskInfo: operation.TaskInfo{ID: "task-3", Status: corestatus.Failed}},
					{TaskInfo: operation.TaskInfo{ID: "task-4", Status: corestatus.Cancelled}},
				},
			},
			{
				OperationID: "3",
				Units: []operation.UnitTaskResult{
					{TaskInfo: operation.TaskInfo{ID: "task-5", Status: corestatus.Completed}},
				},
				Machines: []operation.MachineTaskResult{
					{TaskInfo: operation.TaskInfo{ID: "task-6", Status: corestatus.Completed}},
				},
			},
		},
		Truncated: false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 3)

	// op-1 should have Running status (Running > Completed).
	c.Check(result.Operations[0].OperationID, tc.Equals, "1")
	c.Check(result.Operations[0].Status, tc.Equals, corestatus.Running)

	// op-2 should have Failed status (Failed > Cancelled).
	c.Check(result.Operations[1].OperationID, tc.Equals, "2")
	c.Check(result.Operations[1].Status, tc.Equals, corestatus.Failed)

	// op-3 should have Completed status (all tasks Completed).
	c.Check(result.Operations[2].OperationID, tc.Equals, "3")
	c.Check(result.Operations[2].Status, tc.Equals, corestatus.Completed)
}

func (s *querySuite) TestGetOperationsWithTruncatedResult(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{
		Limit: ptr(1),
	}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "1",
				Units:       []operation.UnitTaskResult{{TaskInfo: operation.TaskInfo{ID: "2", Status: corestatus.Completed}}},
			},
		},
		Truncated: true,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsWithComplexQueryParams(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{
		ActionNames:  []string{"action1", "action2"},
		Applications: []string{"app1", "app2"},
		Status:       []corestatus.Status{corestatus.Running, corestatus.Completed},
		Limit:        ptr(20),
		Offset:       ptr(10),
	}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "42",
				Summary:     "Filtered operation",
				Units:       []operation.UnitTaskResult{{TaskInfo: operation.TaskInfo{ID: "43", Status: corestatus.Running}}},
			},
		},
		Truncated: false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].OperationID, tc.Equals, "42")
	c.Check(result.Operations[0].Status, tc.Equals, corestatus.Running)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsStatusComputationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "42",
				Units: []operation.UnitTaskResult{
					{TaskInfo: operation.TaskInfo{ID: "43", Status: corestatus.Status("unknown-status")}},
				},
			},
		},
		Truncated: false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].OperationID, tc.Equals, "42")
	// Should default to Pending when status computation fails
	c.Check(result.Operations[0].Status, tc.Equals, corestatus.Pending)
}

// TestGetOperationsWithEmptyTaskLists tests operations that have no tasks
// (neither units nor machines).
func (s *querySuite) TestGetOperationsWithEmptyTaskLists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	params := operation.QueryArgs{}

	expectedStateResult := operation.QueryResult{
		Operations: []operation.OperationInfo{
			{
				OperationID: "42",
				Summary:     "Operation with no tasks",
				Units:       []operation.UnitTaskResult{},
				Machines:    []operation.MachineTaskResult{},
			},
		},
		Truncated: false,
	}

	s.state.EXPECT().GetOperations(gomock.Any(), params).Return(expectedStateResult, nil)

	// Act
	result, err := s.service(c).GetOperations(c.Context(), params)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].OperationID, tc.Equals, "42")
	// Empty task list should result in Error status
	c.Check(result.Operations[0].Status, tc.Equals, corestatus.Error)
}

func ptr[T any](v T) *T {
	return &v
}
