// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
)

type querySuite struct {
	baseSuite
}

func TestQuerySuite(t *testing.T) {
	tc.Run(t, &querySuite{})
}

func (s *querySuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

// TestOperationData holds information about test operations for verification

func (s *querySuite) TestGetOperationByIDUnitAndMachineTasks(c *tc.C) {
	// Arrange: create an operation with both unit and machine tasks
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "exec-group")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Add unit task
	unitUUID := s.addUnitWithName(c, charmUUID, "byid-test-app/0")
	taskUUIDUnit := s.addOperationTaskWithID(c, opUUID, "2", "running")
	s.addOperationUnitTask(c, taskUUIDUnit, unitUUID)
	s.addOperationTaskLog(c, taskUUIDUnit, "unit log entry")
	s.addOperationParameter(c, opUUID, "param1", "value1")

	// Add machine task
	machineUUID := s.addMachine(c, "byid-0")
	taskUUIDMachine := s.addOperationTaskWithID(c, opUUID, "3", "completed")
	s.addOperationMachineTask(c, taskUUIDMachine, machineUUID)
	s.addOperationTaskLog(c, taskUUIDMachine, "machine log entry")
	s.addOperationParameter(c, opUUID, "param2", "value2")

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.OperationID, tc.Equals, opID)
	c.Check(info.Enqueued.IsZero(), tc.Equals, false)
	c.Check(info.Started.IsZero(), tc.Equals, true)   // Not yet started.
	c.Check(info.Completed.IsZero(), tc.Equals, true) // Not yet completed.
	c.Assert(info.Units, tc.HasLen, 1)
	c.Assert(info.Machines, tc.HasLen, 1)

	// Unit task checks.
	unitTask := info.Units[0]
	c.Check(unitTask.ActionName, tc.Equals, "test-action")
	c.Check(unitTask.ID, tc.Equals, "2")
	c.Check(unitTask.ReceiverName, tc.Equals, coreunit.Name("byid-test-app/0"))
	c.Check(unitTask.Status.String(), tc.Equals, "running")
	c.Check(unitTask.Parameters["param1"], tc.Equals, "value1")
	c.Assert(unitTask.Log, tc.HasLen, 1)
	c.Check(unitTask.Log[0].Message, tc.Equals, "unit log entry")

	// Machine task checks.
	machineTask := info.Machines[0]
	c.Check(machineTask.ActionName, tc.Equals, "test-action")
	c.Check(machineTask.ID, tc.Equals, "3")
	c.Check(machineTask.ReceiverName, tc.Equals, coremachine.Name("byid-0"))
	c.Check(machineTask.Status.String(), tc.Equals, "completed")
	c.Check(machineTask.Parameters["param2"], tc.Equals, "value2")
	c.Assert(machineTask.Log, tc.HasLen, 1)
	c.Check(machineTask.Log[0].Message, tc.Equals, "machine log entry")
}

func (s *querySuite) TestGetOperationByIDNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetOperationByID(c.Context(), "non-existent-id")

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.OperationNotFound)
	c.Assert(err, tc.ErrorMatches, `operation "non-existent-id" not found`)
}

func (s *querySuite) TestGetOperationsEmptyResult(c *tc.C) {
	// Act - query with no operations in database
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsAllOperations(c *tc.C) {
	// Arrange - create multiple operations.
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "allops-test-app/0")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "completed")
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	machineUUID := s.addMachine(c, "allops-0")
	taskUUID2 := s.addOperationTaskWithID(c, opUUID2, "task-2", "running")
	s.addOperationMachineTask(c, taskUUID2, machineUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 2)
	c.Check(result.Truncated, tc.Equals, false)

	// Check operations are ordered by enqueued_at DESC (newest first).
	op1, op2 := result.Operations[0], result.Operations[1]
	c.Check(op1.Enqueued.After(op2.Enqueued) || op1.Enqueued.Equal(op2.Enqueued), tc.Equals, true)
}

func (s *querySuite) TestGetOperationsFilterByActionNames(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation with "test-action"
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "filterbyaction-test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation with "other-action"
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "other-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "filterbyaction-test-app/1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Act - filter by "test-action"
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"test-action"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ActionName, tc.Equals, "test-action")
}

func (s *querySuite) TestGetOperationsPaginationLimitZero(c *tc.C) {
	// Arrange - create 3 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "limit-zero-app/0")

	for i := 0; i < 3; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - query with limit 0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: func(i int) *int { return &i }(0),
	})

	// Assert - should return empty results but truncated should be false since limit is 0
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsTruncatedFlagLimitLessThanAvailable(c *tc.C) {
	// Arrange - create exactly 5 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "truncated-less-app/0")

	for i := 0; i < 5; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	limit := 3
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 3)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsTruncatedFlagLimitEqualToAvailable(c *tc.C) {
	// Arrange - create exactly 5 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "truncated-equal-app/0")

	for i := 0; i < 5; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	limit := 5
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert - Could be more, so truncated=true
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 5)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsTruncatedFlagLimitGreaterThanAvailable(c *tc.C) {
	// Arrange - create exactly 5 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "truncated-greater-app/0")

	for i := 0; i < 5; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	limit := 10
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 5)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsComplexFilterCombination(c *tc.C) {
	// Arrange - create operations that match complex filter criteria
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Create setup-app1 units
	unitUUID1 := s.addUnitWithName(c, charmUUID, "setup-app1/0")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "setup-app1/1")

	// Operation 1: test-action, setup-app1/0, running status
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "running")
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation 2: test-action, setup-app1/1, pending status
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	taskUUID2 := s.addOperationTaskWithID(c, opUUID2, "task-2", "pending")
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Operation 3: different action (shouldn't match)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")
	opUUID3 := s.addOperation(c)
	s.addOperationAction(c, opUUID3, charmUUID, "other-action")
	taskUUID3 := s.addOperationTaskWithID(c, opUUID3, "task-3", "running")
	s.addOperationUnitTask(c, taskUUID3, unitUUID1)

	// Act - complex filter combining multiple criteria
	limit := 10
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames:  []string{"test-action"},
		Applications: []string{"setup-app1"},
		Status:       []corestatus.Status{corestatus.Running, corestatus.Pending},
		Units:        []unit.Name{"setup-app1/0", "setup-app1/1"},
		Limit:        &limit,
	})

	// Assert - should return operations that match ALL criteria
	c.Assert(err, tc.IsNil)
	// Should find the running unit task from setup-app1/0 and pending unit task from setup-app1/1
	c.Assert(result.Operations, tc.HasLen, 2)

	for _, op := range result.Operations {
		c.Check(op.Units[0].ActionName, tc.Equals, "test-action")
		c.Check(op.Units[0].Status, tc.Satisfies, func(status corestatus.Status) bool {
			return status == corestatus.Running || status == corestatus.Pending
		})
	}
}

func (s *querySuite) TestGetOperationsEmptyActionNamesSlice(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "empty-actions-app/0")

	for i := 0; i < 4; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{ActionNames: []string{}})

	// Assert - Should return all operations
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 4)
}

func (s *querySuite) TestGetOperationsEmptyApplicationsSlice(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "empty-apps-app/0")

	for i := 0; i < 4; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{Applications: []string{}})

	// Assert - Should return all operations
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 4)
}

func (s *querySuite) TestGetOperationsEmptyStatusSlice(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "empty-status-app/0")

	for i := 0; i < 4; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{Status: []corestatus.Status{}})

	// Assert - Should return all operations
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 4)
}

func (s *querySuite) TestGetOperationsNonExistentAction(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "nonexist-action-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"non-existent-action"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNonExistentApplication(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "nonexist-app-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"non-existent-app"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNonExistentUnit(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "nonexist-unit-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Units: []unit.Name{"non-existent-app/0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNonExistentMachine(c *tc.C) {
	// Arrange - create test operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "nonexist-machine-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Machines: []machine.Name{"non-existent-machine"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNonMatchingStatus(c *tc.C) {
	// Arrange - create test operations with running status
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "nonmatch-status-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTaskWithID(c, opUUID, "task-1", "running")
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - filter by error status which doesn't exist
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Status: []corestatus.Status{corestatus.Error},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsOrderVerification(c *tc.C) {
	// Arrange - create operations with known timing
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "ordertest-app/0")

	var operationIDs []string

	// Create 3 operations with slight delays to ensure different timestamps
	for i := 0; i < 3; i++ {
		opUUID := s.addOperationWithExecutionGroup(c, fmt.Sprintf("order-test-%d", i))
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)

		opID := s.selectDistinctValues(c, "operation_id", "operation")[0]
		operationIDs = append(operationIDs, opID)

		// Small delay to ensure different enqueued_at timestamps
		time.Sleep(time.Millisecond)
	}

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{})

	// Assert - should be ordered by enqueued_at DESC (newest first)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 3)

	// Verify chronological order (newest first)
	for i := 0; i < len(result.Operations)-1; i++ {
		current := result.Operations[i].Enqueued
		next := result.Operations[i+1].Enqueued
		c.Check(current.After(next) || current.Equal(next), tc.Equals, true,
			tc.Commentf("Operation %d should be newer than operation %d", i, i+1))
	}
}

func (s *querySuite) TestGetOperationByIDWithTimestamps(c *tc.C) {
	// Arrange - create operation with started and completed timestamps
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "timestamp-test")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	// Add timestamps to the operation
	now := s.state.clock.Now()
	startedAt := now.Add(-time.Hour)
	completedAt := now.Add(-time.Minute)

	s.query(c, `UPDATE operation SET started_at = ?, completed_at = ? WHERE uuid = ?`,
		startedAt, completedAt, opUUID)

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.OperationID, tc.Equals, opID)
	c.Check(info.Enqueued.IsZero(), tc.Equals, false)
	c.Check(info.Started.IsZero(), tc.Equals, false)
	c.Check(info.Completed.IsZero(), tc.Equals, false)

	// Verify timestamp precision (within reasonable delta due to database precision)
	c.Check(info.Started.Unix(), tc.Equals, startedAt.Unix())
	c.Check(info.Completed.Unix(), tc.Equals, completedAt.Unix())
}

func (s *querySuite) TestGetOperationByIDWithSummary(c *tc.C) {
	// Arrange - create operation with a summary
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "summary-test")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	expectedSummary := "Test operation summary"
	s.query(c, `UPDATE operation SET summary = ? WHERE uuid = ?`, expectedSummary, opUUID)

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Summary, tc.Equals, expectedSummary)
}

func (s *querySuite) TestGetOperationByIDNoTasks(c *tc.C) {
	// Arrange - create operation with no tasks
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "no-tasks")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	// Note: intentionally not adding any tasks

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.OperationID, tc.Equals, opID)
	c.Assert(info.Units, tc.HasLen, 0)
	c.Assert(info.Machines, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationByIDEmptyString(c *tc.C) {
	// Act
	_, err := s.state.GetOperationByID(c.Context(), "")

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.OperationNotFound)
	c.Assert(err, tc.ErrorMatches, `operation "" not found`)
}

func (s *querySuite) TestGetOperationByIDNonExistentID(c *tc.C) {
	// Act
	_, err := s.state.GetOperationByID(c.Context(), "non-existent-operation-id")

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.OperationNotFound)
	c.Assert(err, tc.ErrorMatches, `operation "non-existent-operation-id" not found`)
}

func (s *querySuite) TestGetOperationByIDUUIDFormatNonExistent(c *tc.C) {
	// Act
	_, err := s.state.GetOperationByID(c.Context(), "550e8400-e29b-41d4-a716-446655440000")

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.OperationNotFound)
	c.Assert(err, tc.ErrorMatches, `operation "550e8400-e29b-41d4-a716-446655440000" not found`)
}

func (s *querySuite) TestGetOperationsWithManyTasks(c *tc.C) {
	// Arrange - create operation with multiple unit and machine tasks
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "many-tasks")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	// Create multiple units and machines
	var unitUUIDs []string
	var machineUUIDs []string

	for i := 0; i < 3; i++ {
		unitUUID := s.addUnitWithName(c, charmUUID, fmt.Sprintf("manytasks-app/unit-%d", i))
		unitUUIDs = append(unitUUIDs, unitUUID)

		machineUUID := s.addMachine(c, fmt.Sprintf("manytasks-machine-%d", i))
		machineUUIDs = append(machineUUIDs, machineUUID)
	}

	// Create tasks for each unit and machine
	for i, unitUUID := range unitUUIDs {
		taskUUID := s.addOperationTaskWithID(c, opUUID, fmt.Sprintf("unit-task-%d", i), "running")
		s.addOperationUnitTask(c, taskUUID, unitUUID)
		s.addOperationTaskLog(c, taskUUID, fmt.Sprintf("Unit task %d log", i))
		s.addOperationParameter(c, opUUID, fmt.Sprintf("unit-param-%d", i), fmt.Sprintf("unit-value-%d", i))
	}

	for i, machineUUID := range machineUUIDs {
		taskUUID := s.addOperationTaskWithID(c, opUUID, fmt.Sprintf("machine-task-%d", i), "completed")
		s.addOperationMachineTask(c, taskUUID, machineUUID)
		s.addOperationTaskLog(c, taskUUID, fmt.Sprintf("Machine task %d log", i))
		s.addOperationParameter(c, opUUID, fmt.Sprintf("machine-param-%d", i), fmt.Sprintf("machine-value-%d", i))
	}

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Units, tc.HasLen, 3)
	c.Assert(info.Machines, tc.HasLen, 3)

	// Verify all unit tasks
	for i, unitTask := range info.Units {
		c.Check(unitTask.ID, tc.Equals, fmt.Sprintf("unit-task-%d", i))
		c.Check(unitTask.Status, tc.Equals, corestatus.Running)
		c.Assert(unitTask.Log, tc.HasLen, 1)
		c.Check(unitTask.Log[0].Message, tc.Equals, fmt.Sprintf("Unit task %d log", i))
		c.Check(unitTask.Parameters[fmt.Sprintf("unit-param-%d", i)], tc.Equals, fmt.Sprintf("unit-value-%d", i))
	}

	// Verify all machine tasks
	for i, machineTask := range info.Machines {
		c.Check(machineTask.ID, tc.Equals, fmt.Sprintf("machine-task-%d", i))
		c.Check(machineTask.Status, tc.Equals, corestatus.Completed)
		c.Assert(machineTask.Log, tc.HasLen, 1)
		c.Check(machineTask.Log[0].Message, tc.Equals, fmt.Sprintf("Machine task %d log", i))
		c.Check(machineTask.Parameters[fmt.Sprintf("machine-param-%d", i)], tc.Equals, fmt.Sprintf("machine-value-%d", i))
	}
}

func (s *querySuite) TestGetOperationsWithOffsetDefaultLimitOffset0(c *tc.C) {
	// Arrange - create exactly 15 operations for offset testing
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "offset-default-0-app/0")

	for i := 0; i < 15; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	offset := 0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Offset: &offset,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 10)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsWithOffsetDefaultLimitOffset5(c *tc.C) {
	// Arrange - create exactly 15 operations for offset testing
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "offset-default-5-app/0")

	for i := 0; i < 15; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	offset := 5
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Offset: &offset,
	})

	// Assert - Still 10 results at limit, could be more
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 10)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsWithOffsetDefaultLimitOffset10(c *tc.C) {
	// Arrange - create exactly 15 operations for offset testing
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "offset-default-10-app/0")

	for i := 0; i < 15; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	offset := 10
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Offset: &offset,
	})

	// Assert - Only 5 left
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 5)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsWithOffsetCustomLimitWithOffset(c *tc.C) {
	// Arrange - create exactly 15 operations for offset testing
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "offset-custom-app/0")

	for i := 0; i < 15; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act
	limit := 7
	offset := 3
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit:  &limit,
		Offset: &offset,
	})

	// Assert - 7 out of remaining 12
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 7)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationsLimitNegative(c *tc.C) {
	// Arrange - create a few operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "limit-negative-app/0")

	for i := 0; i < 3; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - Negative limit returns all available
	limit := -5
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 3)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsLimitVeryLarge(c *tc.C) {
	// Arrange - create a few operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "limit-large-app/0")

	for i := 0; i < 3; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - Should get all 3, not truncated
	limit := 1000
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 3)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsLimitOne(c *tc.C) {
	// Arrange - create a few operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "limit-one-app/0")

	for i := 0; i < 3; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - Should get 1, could be more
	limit := 1
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Truncated, tc.Equals, true)
}

func (s *querySuite) TestGetOperationByIDOperationWithoutParameters(c *tc.C) {
	// Arrange - create operation with tasks but no parameters
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "no-params")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	// Add unit task but no parameters
	unitUUID := s.addUnitWithName(c, charmUUID, "noparams-test-app/0")
	taskUUID := s.addOperationTaskWithID(c, opUUID, "task-1", "running")
	s.addOperationUnitTask(c, taskUUID, unitUUID)
	s.addOperationTaskLog(c, taskUUID, "task log without parameters")
	// Note: intentionally not adding any parameters

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Units, tc.HasLen, 1)
	c.Check(info.Units[0].Parameters, tc.HasLen, 0)
	c.Assert(info.Units[0].Log, tc.HasLen, 1)
	c.Check(info.Units[0].Log[0].Message, tc.Equals, "task log without parameters")
}

func (s *querySuite) TestGetOperationByIDOperationWithoutLogs(c *tc.C) {
	// Arrange - create operation with tasks but no logs
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "no-logs")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	// Add unit task with parameters but no logs
	unitUUID := s.addUnitWithName(c, charmUUID, "nologs-test-app/0")
	taskUUID := s.addOperationTaskWithID(c, opUUID, "task-1", "running")
	s.addOperationUnitTask(c, taskUUID, unitUUID)
	s.addOperationParameter(c, opUUID, "param1", "value1")
	// Note: intentionally not adding any logs

	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Act
	info, err := s.state.GetOperationByID(c.Context(), opID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Units, tc.HasLen, 1)
	c.Check(info.Units[0].Parameters["param1"], tc.Equals, "value1")
	c.Check(info.Units[0].Log, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsSQLInjectionActionName(c *tc.C) {
	// Arrange - create test operation
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "sql-inject-action-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - SQL injection attempt in action name
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"'; DROP TABLE operation; --"},
	})

	// Assert - Should not panic or cause SQL errors
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsSQLInjectionApplicationName(c *tc.C) {
	// Arrange - create test operation
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "sql-inject-app-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - SQL injection attempt in application name
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"app'; DELETE FROM operation; --"},
	})

	// Assert - Should not panic or cause SQL errors
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsSQLInjectionUnitName(c *tc.C) {
	// Arrange - create test operation
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "sql-inject-unit-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - SQL injection attempt in unit name
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Units: []unit.Name{"unit/'; TRUNCATE operation; --"},
	})

	// Assert - Should not panic or cause SQL errors
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsSQLInjectionMachineName(c *tc.C) {
	// Arrange - create test operation
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "sql-inject-machine-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - SQL injection attempt in machine name
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Machines: []machine.Name{"machine'; DROP DATABASE; --"},
	})

	// Assert - Should not panic or cause SQL errors
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsVeryLongStrings(c *tc.C) {
	// Arrange - create test operation
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "long-strings-app/0")

	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act - very long strings
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames:  []string{strings.Repeat("a", 10000)},
		Applications: []string{strings.Repeat("b", 10000)},
	})

	// Assert - Should not panic or cause SQL errors
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
}

func (s *querySuite) TestGetOperationsStatusFilterAllValues(c *tc.C) {
	// Arrange - create operations with different statuses
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "statustest-app/0")

	// Create operations with all possible status values
	statuses := []string{"pending", "running", "completed", "cancelled", "aborting", "error"}
	for _, status := range statuses {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTaskWithID(c, opUUID, "task-"+status, status)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Test filtering by each status
	for _, status := range statuses {
		var coreStatus corestatus.Status
		switch status {
		case "pending":
			coreStatus = corestatus.Pending
		case "running":
			coreStatus = corestatus.Running
		case "completed":
			coreStatus = corestatus.Completed
		case "cancelled":
			coreStatus = corestatus.Cancelled
		case "aborting":
			coreStatus = corestatus.Aborting
		case "error":
			coreStatus = corestatus.Error
		}

		result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
			Status: []corestatus.Status{coreStatus},
		})

		c.Assert(err, tc.IsNil, tc.Commentf("Status: %s", status))
		c.Assert(result.Operations, tc.HasLen, 1, tc.Commentf("Status: %s", status))
		c.Check(result.Operations[0].Units[0].Status, tc.Equals, coreStatus, tc.Commentf("Status: %s", status))
	}
}

func (s *querySuite) TestGetOperationsMultipleStatusFilter(c *tc.C) {
	// Arrange - create operations with different statuses
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "multistat-app/0")

	// Create operations with running and completed statuses
	statuses := []string{"running", "completed", "pending"}
	for _, status := range statuses {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTaskWithID(c, opUUID, "task-"+status, status)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - filter by multiple statuses
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Status: []corestatus.Status{corestatus.Running, corestatus.Completed},
	})

	// Assert - should return operations with either status
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 2)

	foundStatuses := make(map[corestatus.Status]bool)
	for _, op := range result.Operations {
		foundStatuses[op.Units[0].Status] = true
	}
	c.Check(foundStatuses[corestatus.Running], tc.Equals, true)
	c.Check(foundStatuses[corestatus.Completed], tc.Equals, true)
}

func (s *querySuite) TestGetOperationsFilterByMultipleActionNames(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "third-action")

	// Operation with "test-action"
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "filtermultiaction-test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation with "other-action"
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "other-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "filtermultiaction-test-app/1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Operation with "third-action"
	opUUID3 := s.addOperation(c)
	s.addOperationAction(c, opUUID3, charmUUID, "third-action")
	unitUUID3 := s.addUnitWithName(c, charmUUID, "filtermultiaction-test-app/2")
	taskUUID3 := s.addOperationTask(c, opUUID3)
	s.addOperationUnitTask(c, taskUUID3, unitUUID3)

	// Act - filter by multiple action names
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"test-action", "other-action"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 2)
	actionNames := []string{result.Operations[0].Units[0].ActionName, result.Operations[1].Units[0].ActionName}
	c.Check(actionNames, tc.DeepEquals, []string{"other-action", "test-action"}) // Ordered by enqueued_at DESC
}

func (s *querySuite) TestGetOperationsFilterByStatus(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "filterstatus-test-app/0")

	// Operation with completed task
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "completed")
	s.addOperationUnitTask(c, taskUUID1, unitUUID)

	// Operation with running task
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	taskUUID2 := s.addOperationTaskWithID(c, opUUID2, "task-2", "running")
	s.addOperationUnitTask(c, taskUUID2, unitUUID)

	// Act - filter by completed status
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Status: []corestatus.Status{corestatus.Completed},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].Status, tc.Equals, corestatus.Completed)
}

func (s *querySuite) TestGetOperationsFilterByUnits(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation targeting filterunits-test-app/0
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "filterunits-test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation targeting filterunits-test-app/1
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "filterunits-test-app/1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Act - filter by unit filterunits-test-app/0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Units: []coreunit.Name{"filterunits-test-app/0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ReceiverName, tc.Equals, coreunit.Name("filterunits-test-app/0"))
}

func (s *querySuite) TestGetOperationsFilterByMachines(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation targeting machine filtermachines-0
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	machineUUID1 := s.addMachine(c, "filtermachines-0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationMachineTask(c, taskUUID1, machineUUID1)

	// Operation targeting machine filtermachines-1
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	machineUUID2 := s.addMachine(c, "filtermachines-1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationMachineTask(c, taskUUID2, machineUUID2)

	// Act - filter by machine filtermachines-0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Machines: []coremachine.Name{"filtermachines-0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Machines[0].ReceiverName, tc.Equals, coremachine.Name("filtermachines-0"))
}

func (s *querySuite) TestGetOperationsFilterByApplications(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation targeting filterapps-app1
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "filterapps-app1/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation targeting filterapps-app2 with different charm
	charmUUID2 := s.addCharm(c)
	s.addCharmAction(c, charmUUID2)
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID2, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID2, "filterapps-app2/0")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Act - filter by filterapps-app1
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"filterapps-app1"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ReceiverName, tc.Equals, coreunit.Name("filterapps-app1/0"))
}

func (s *querySuite) TestGetOperationsCombinedFilters(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")

	// Operation 1: test-action on combined-app1/0, completed
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "combined-app1/0")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "completed")
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation 2: test-action on combined-app1/1, running
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "combined-app1/1")
	taskUUID2 := s.addOperationTaskWithID(c, opUUID2, "task-2", "running")
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Operation 3: other-action on combined-app1/0, completed
	opUUID3 := s.addOperation(c)
	s.addOperationAction(c, opUUID3, charmUUID, "other-action")
	taskUUID3 := s.addOperationTaskWithID(c, opUUID3, "task-3", "completed")
	s.addOperationUnitTask(c, taskUUID3, unitUUID1)

	// Act - filter by test-action AND combined-app1 AND completed status
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames:  []string{"test-action"},
		Applications: []string{"combined-app1"},
		Status:       []corestatus.Status{corestatus.Completed},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ActionName, tc.Equals, "test-action")
	c.Check(result.Operations[0].Units[0].Status, tc.Equals, corestatus.Completed)
	c.Check(result.Operations[0].Units[0].ReceiverName, tc.Equals, coreunit.Name("combined-app1/0"))
}

func (s *querySuite) TestGetOperationsPagination(c *tc.C) {
	// Arrange - create 5 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "pagination-test-app/0")

	for i := 0; i < 5; i++ {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - get first 2 operations
	limit := 2
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 2)
	c.Check(result.Truncated, tc.Equals, true)

	// Act - get next 2 operations
	offset := 2
	result2, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit:  &limit,
		Offset: &offset,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(result2.Operations, tc.HasLen, 2)
	c.Check(result2.Truncated, tc.Equals, true)

	// Act - get last operation
	offset = 4
	result3, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit:  &limit,
		Offset: &offset,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(result3.Operations, tc.HasLen, 1)
	c.Check(result3.Truncated, tc.Equals, false) // Less than limit, so not truncated

	// Verify no duplicates between pages
	allOpIDs := make(map[string]bool)
	for _, op := range append(append(result.Operations, result2.Operations...), result3.Operations...) {
		c.Check(allOpIDs[op.OperationID], tc.Equals, false, tc.Commentf("Duplicate operation ID: %s", op.OperationID))
		allOpIDs[op.OperationID] = true
	}
	c.Check(allOpIDs, tc.HasLen, 5)
}

func (s *querySuite) TestGetOperationsPaginationEdgeCases(c *tc.C) {
	// Act - test with offset beyond available data
	offset := 1000
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Offset: &offset,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)

	// Act - test with zero limit
	limit := 0
	result2, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Limit: &limit,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(result2.Operations, tc.HasLen, 0)
	c.Check(result2.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsCompleteDataVerification(c *tc.C) {
	// Arrange - create operation with all data types
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "test-group")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")

	// Add unit task with parameters and logs
	unitUUID := s.addUnitWithName(c, charmUUID, "completedata-test-app/0")
	taskUUIDUnit := s.addOperationTaskWithID(c, opUUID, "unit-task", "running")
	s.addOperationUnitTask(c, taskUUIDUnit, unitUUID)
	s.addOperationTaskLog(c, taskUUIDUnit, "unit log message")
	s.addOperationParameter(c, opUUID, "unit-param", "unit-value")

	// Add machine task with parameters and logs
	machineUUID := s.addMachine(c, "completedata-0")
	taskUUIDMachine := s.addOperationTaskWithID(c, opUUID, "machine-task", "completed")
	s.addOperationMachineTask(c, taskUUIDMachine, machineUUID)
	s.addOperationTaskLog(c, taskUUIDMachine, "machine log message")
	s.addOperationParameter(c, opUUID, "machine-param", "machine-value")

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)

	op := result.Operations[0]
	c.Check(op.OperationID, tc.Not(tc.Equals), "")
	c.Check(op.Enqueued.IsZero(), tc.Equals, false)

	// Verify unit task
	c.Assert(op.Units, tc.HasLen, 1)
	unitTask := op.Units[0]
	c.Check(unitTask.ID, tc.Equals, "unit-task")
	c.Check(unitTask.ActionName, tc.Equals, "test-action")
	c.Check(unitTask.Status, tc.Equals, corestatus.Running)
	c.Check(unitTask.ReceiverName, tc.Equals, coreunit.Name("completedata-test-app/0"))
	c.Check(unitTask.Parameters["unit-param"], tc.Equals, "unit-value")
	c.Assert(unitTask.Log, tc.HasLen, 1)
	c.Check(unitTask.Log[0].Message, tc.Equals, "unit log message")

	// Verify machine task
	c.Assert(op.Machines, tc.HasLen, 1)
	machineTask := op.Machines[0]
	c.Check(machineTask.ID, tc.Equals, "machine-task")
	c.Check(machineTask.ActionName, tc.Equals, "test-action")
	c.Check(machineTask.Status, tc.Equals, corestatus.Completed)
	c.Check(machineTask.ReceiverName, tc.Equals, coremachine.Name("completedata-0"))
	c.Check(machineTask.Parameters["machine-param"], tc.Equals, "machine-value")
	c.Assert(machineTask.Log, tc.HasLen, 1)
	c.Check(machineTask.Log[0].Message, tc.Equals, "machine log message")
}

func (s *querySuite) TestGetOperationsNoMatchingFiltersNonExistentAction(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "nomatch-action-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"non-existent-action"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNoMatchingFiltersNonExistentUnit(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "nomatch-unit-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Units: []coreunit.Name{"non-existent/0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNoMatchingFiltersNonExistentMachine(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "nomatch-machine-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Machines: []coremachine.Name{"999"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNoMatchingFiltersNonExistentApplication(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "nomatch-appfilter-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"non-existent-app"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

func (s *querySuite) TestGetOperationsNoMatchingFiltersNonExistentStatus(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "nomatch-status-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	// Act
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Status: []corestatus.Status{corestatus.Error},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 0)
	c.Check(result.Truncated, tc.Equals, false)
}

// TestGetOperations_PaginationWithActionFilter verifies that pagination works
// correctly when combined with an action name filter. It creates multiple
// operations tagged with two different actions and then pages through the
// filtered result set using limit+offset.
func (s *querySuite) TestGetOperationsPaginationWithActionFilter(c *tc.C) {
	// Arrange - create 5 operations: 3 with "test-action" and 2 with
	// "other-action".
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "paginationaction-test-app/0")

	// Create 3 operations with "test-action"
	for range 3 {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "test-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Create 2 operations with "other-action"
	for range 2 {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "other-action")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitUUID)
	}

	// Act - get first page (limit=2) for "test-action"
	limit := 2
	resultPage1, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"test-action"},
		Limit:       &limit,
	})
	c.Assert(err, tc.IsNil)
	c.Check(resultPage1.Operations, tc.HasLen, 2)
	// There are 3 operations matching the filter, so the first page (len==limit)
	// should be marked truncated.
	c.Check(resultPage1.Truncated, tc.Equals, true)
	for _, op := range resultPage1.Operations {
		c.Check(op.Units[0].ActionName, tc.Equals, "test-action")
	}

	// Act - get second page (offset=2, limit=2)
	offset := 2
	resultPage2, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames: []string{"test-action"},
		Limit:       &limit,
		Offset:      &offset,
	})
	c.Assert(err, tc.IsNil)
	// Only one remaining operation should be returned.
	c.Check(resultPage2.Operations, tc.HasLen, 1)
	// Since fewer than limit were returned, truncated must be false.
	c.Check(resultPage2.Truncated, tc.Equals, false)
	c.Check(resultPage2.Operations[0].Units[0].ActionName, tc.Equals, "test-action")

	// Verify that concatenating pages yields unique operation IDs and matches
	// the expected count.
	allOpIDs := make(map[string]bool)
	for _, op := range append(resultPage1.Operations, resultPage2.Operations...) {
		c.Check(allOpIDs[op.OperationID], tc.Equals, false, tc.Commentf("Duplicate operation ID: %s", op.OperationID))
		allOpIDs[op.OperationID] = true
	}
	c.Check(allOpIDs, tc.HasLen, 3)
}

// TestGetOperations_PaginationWithCombinedFilters verifies pagination works
// in combination with receiver filters (applications) and offset. This ensures
// that LIMIT/OFFSET are applied to the filtered result set.
func (s *querySuite) TestGetOperationsPaginationWithCombinedFilters(c *tc.C) {
	// Arrange - create operations across two applications
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// App "alpha" -> 3 operations
	unitAlpha := s.addUnitWithName(c, charmUUID, "alpha/0")
	for range 3 {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID, "deploy")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitAlpha)
	}

	// App "beta" -> 2 operations
	charmUUID2 := s.addCharm(c)
	s.addCharmAction(c, charmUUID2)
	unitBeta := s.addUnitWithName(c, charmUUID2, "beta/0")
	for range 2 {
		opUUID := s.addOperation(c)
		s.addOperationAction(c, opUUID, charmUUID2, "deploy")
		taskUUID := s.addOperationTask(c, opUUID)
		s.addOperationUnitTask(c, taskUUID, unitBeta)
	}

	// Act - query for application "alpha" with limit=1 and offset=1 (should
	// get the 2nd of 3).
	limit := 1
	offset := 1
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"alpha"},
		Limit:        &limit,
		Offset:       &offset,
	})
	c.Assert(err, tc.IsNil)
	// Expect exactly 1 operation returned
	c.Assert(result.Operations, tc.HasLen, 1)
	// There are more alpha operations beyond this page, so truncated should be
	// true.
	c.Check(result.Truncated, tc.Equals, true)
	// Ensure the returned operation targets "alpha"
	c.Check(result.Operations[0].Units[0].ReceiverName.String(), tc.Matches, "alpha/.*")
}
