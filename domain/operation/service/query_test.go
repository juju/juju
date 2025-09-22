// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	context "context"
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
	ids, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

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
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, corestatus.Running)

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
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

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
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

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

func (s *querySuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter)
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

	got, err := s.service().GetOperationByID(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(got.OperationID, tc.Equals, operationID)
	c.Check(got.Status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestGetOperationByID_Completed(c *tc.C) {
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

	got, err := s.service().GetOperationByID(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(got.OperationID, tc.Equals, operationID)
	c.Check(got.Status, tc.Equals, corestatus.Completed)
}

func (s *querySuite) TestGetOperationByID_Error(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedErr := errors.New("not found")
	s.state.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Return(operation.OperationInfo{}, expectedErr)

	_, err := s.service().GetOperationByID(context.Background(), "unknown")
	c.Assert(err, tc.ErrorMatches, "not found")
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
	status := operationStatus(tasks)
	// Should pick Running (highest priority among present)
	c.Assert(status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestOperationStatus_EmptyTasks(c *tc.C) {
	status := operationStatus(nil)
	c.Assert(status, tc.Equals, corestatus.Pending)

	status = operationStatus([]operation.TaskInfo{})
	c.Assert(status, tc.Equals, corestatus.Pending)
}

func (s *querySuite) TestOperationStatus_SingleStatus(c *tc.C) {
	for _, st := range statusOrder {
		tasks := []operation.TaskInfo{{Status: st}}
		status := operationStatus(tasks)
		c.Check(status, tc.Equals, st)
	}
}

func (s *querySuite) TestOperationStatus_MixedStatuses(c *tc.C) {
	// Highest priority is Error, then Running, etc.
	tasks := []operation.TaskInfo{
		{Status: corestatus.Completed},
		{Status: corestatus.Pending},
		{Status: corestatus.Error},
		{Status: corestatus.Running},
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Error)

	// Remove Error, next highest is Running.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Completed},
		{Status: corestatus.Pending},
		{Status: corestatus.Running},
	}
	status = operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Running)

	// Remove Running, next highest is Pending.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Completed},
		{Status: corestatus.Pending},
	}
	status = operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Pending)

	// Remove Pending, next highest is Failed.
	tasks = []operation.TaskInfo{
		{Status: corestatus.Completed},
		{Status: corestatus.Failed},
	}
	status = operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Failed)
}

func (s *querySuite) TestOperationStatus_AllStatusesPresent(c *tc.C) {
	// If all statuses are present, should return Error (highest priority).
	tasks := []operation.TaskInfo{}
	for _, st := range statusOrder {
		tasks = append(tasks, operation.TaskInfo{Status: st})
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Error)
}

func (s *querySuite) TestOperationStatus_DuplicateStatuses(c *tc.C) {
	// Multiple tasks with same status, should still return that status.
	tasks := []operation.TaskInfo{
		{Status: corestatus.Failed},
		{Status: corestatus.Failed},
		{Status: corestatus.Failed},
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Failed)
}

func (s *querySuite) TestOperationStatus_UnknownStatus(c *tc.C) {
	// Simulate a status not in statusOrder.
	type fakeStatus string
	const unknownStatus fakeStatus = "unknown"
	tasks := []operation.TaskInfo{
		{Status: corestatus.Status(unknownStatus)},
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Pending)
}

func (s *querySuite) TestOperationStatus_MixedKnownAndUnknown(c *tc.C) {
	// If at least one known status is present, should pick highest priority
	// known.
	type fakeStatus string
	const unknownStatus fakeStatus = "unknown"
	tasks := []operation.TaskInfo{
		{Status: corestatus.Status(unknownStatus)},
		{Status: corestatus.Running},
		{Status: corestatus.Completed},
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Running)
}

func (s *querySuite) TestOperationStatus_OnlyUnknownStatuses(c *tc.C) {
	type fakeStatus string
	const unknownStatus fakeStatus = "unknown"
	tasks := []operation.TaskInfo{
		{Status: corestatus.Status(unknownStatus)},
		{Status: corestatus.Status(unknownStatus)},
	}
	status := operationStatus(tasks)
	c.Assert(status, tc.Equals, corestatus.Pending)
}
