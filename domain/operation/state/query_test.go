// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
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

func (s *querySuite) TestGetOperationByIDUnitAndMachineTasks(c *tc.C) {
	// Arrange: create an operation with both unit and machine tasks
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperationWithExecutionGroup(c, "exec-group")
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	opID := s.selectDistinctValues(c, "operation_id", "operation")[0]

	// Add unit task
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUIDUnit := s.addOperationTaskWithID(c, opUUID, "unit-task-id", "running")
	s.addOperationUnitTask(c, taskUUIDUnit, unitUUID)
	s.addOperationTaskLog(c, taskUUIDUnit, "unit log entry")
	s.addOperationParameter(c, opUUID, "param1", "value1")

	// Add machine task
	machineUUID := s.addMachine(c, "0")
	taskUUIDMachine := s.addOperationTaskWithID(c, opUUID, "machine-task-id", "completed")
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
	c.Check(unitTask.ID, tc.Equals, "unit-task-id")
	c.Check(unitTask.ReceiverName, tc.Equals, coreunit.Name("test-app/0"))
	c.Check(unitTask.Status.String(), tc.Equals, "running")
	c.Check(unitTask.Parameters["param1"], tc.Equals, "value1")
	c.Check(len(unitTask.Log), tc.Equals, 1)
	c.Check(unitTask.Log[0].Message, tc.Equals, "unit log entry")

	// Machine task checks.
	machineTask := info.Machines[0]
	c.Check(machineTask.ActionName, tc.Equals, "test-action")
	c.Check(machineTask.ID, tc.Equals, "machine-task-id")
	c.Check(machineTask.ReceiverName, tc.Equals, coremachine.Name("0"))
	c.Check(machineTask.Status.String(), tc.Equals, "completed")
	c.Check(machineTask.Parameters["param2"], tc.Equals, "value2")
	c.Check(len(machineTask.Log), tc.Equals, 1)
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
	// Arrange - create multiple operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation 1
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "completed")
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation 2
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	machineUUID := s.addMachine(c, "0")
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
	unitUUID1 := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation with "other-action"
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "other-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "test-app/1")
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

func (s *querySuite) TestGetOperationsFilterByMultipleActionNames(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "third-action")

	// Operation with "test-action"
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation with "other-action"
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "other-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "test-app/1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Operation with "third-action"
	opUUID3 := s.addOperation(c)
	s.addOperationAction(c, opUUID3, charmUUID, "third-action")
	unitUUID3 := s.addUnitWithName(c, charmUUID, "test-app/2")
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
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")

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

	// Operation targeting test-app/0
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation targeting test-app/1
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "test-app/1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Act - filter by unit test-app/0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Units: []coreunit.Name{"test-app/0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ReceiverName, tc.Equals, coreunit.Name("test-app/0"))
}

func (s *querySuite) TestGetOperationsFilterByMachines(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation targeting machine 0
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	machineUUID1 := s.addMachine(c, "0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationMachineTask(c, taskUUID1, machineUUID1)

	// Operation targeting machine 1
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	machineUUID2 := s.addMachine(c, "1")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationMachineTask(c, taskUUID2, machineUUID2)

	// Act - filter by machine 0
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Machines: []coremachine.Name{"0"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Machines[0].ReceiverName, tc.Equals, coremachine.Name("0"))
}

func (s *querySuite) TestGetOperationsFilterByApplications(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	// Operation targeting app1
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "app1/0")
	taskUUID1 := s.addOperationTask(c, opUUID1)
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation targeting app2
	charmUUID2 := s.addCharm(c)
	s.addCharmAction(c, charmUUID2)
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID2, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID2, "app2/0")
	taskUUID2 := s.addOperationTask(c, opUUID2)
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Act - filter by app1
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		Applications: []string{"app1"},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(string(result.Operations[0].Units[0].ReceiverName), tc.Matches, "app1/.*")
}

func (s *querySuite) TestGetOperationsCombinedFilters(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "other-action")

	// Operation 1: test-action on app1/0, completed
	opUUID1 := s.addOperation(c)
	s.addOperationAction(c, opUUID1, charmUUID, "test-action")
	unitUUID1 := s.addUnitWithName(c, charmUUID, "app1/0")
	taskUUID1 := s.addOperationTaskWithID(c, opUUID1, "task-1", "completed")
	s.addOperationUnitTask(c, taskUUID1, unitUUID1)

	// Operation 2: test-action on app1/1, running
	opUUID2 := s.addOperation(c)
	s.addOperationAction(c, opUUID2, charmUUID, "test-action")
	unitUUID2 := s.addUnitWithName(c, charmUUID, "app1/1")
	taskUUID2 := s.addOperationTaskWithID(c, opUUID2, "task-2", "running")
	s.addOperationUnitTask(c, taskUUID2, unitUUID2)

	// Operation 3: other-action on app1/0, completed
	opUUID3 := s.addOperation(c)
	s.addOperationAction(c, opUUID3, charmUUID, "other-action")
	taskUUID3 := s.addOperationTaskWithID(c, opUUID3, "task-3", "completed")
	s.addOperationUnitTask(c, taskUUID3, unitUUID1)

	// Act - filter by test-action AND app1 AND completed status
	result, err := s.state.GetOperations(c.Context(), operation.QueryArgs{
		ActionNames:  []string{"test-action"},
		Status:       []corestatus.Status{corestatus.Completed},
		Applications: []string{"app1"},
	})

	// Assert - should only return operation 1
	c.Assert(err, tc.IsNil)
	c.Assert(result.Operations, tc.HasLen, 1)
	c.Check(result.Operations[0].Units[0].ActionName, tc.Equals, "test-action")
	c.Check(result.Operations[0].Units[0].Status, tc.Equals, corestatus.Completed)
	c.Check(result.Operations[0].Units[0].ReceiverName.String(), tc.Matches, "app1/.*")
}

func (s *querySuite) TestGetOperationsPagination(c *tc.C) {
	// Arrange - create 5 operations
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")

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
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUIDUnit := s.addOperationTaskWithID(c, opUUID, "unit-task", "running")
	s.addOperationUnitTask(c, taskUUIDUnit, unitUUID)
	s.addOperationTaskLog(c, taskUUIDUnit, "unit log message")
	s.addOperationParameter(c, opUUID, "unit-param", "unit-value")

	// Add machine task with parameters and logs
	machineUUID := s.addMachine(c, "0")
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
	c.Check(unitTask.ReceiverName, tc.Equals, coreunit.Name("test-app/0"))
	c.Check(unitTask.Parameters["unit-param"], tc.Equals, "unit-value")
	c.Assert(unitTask.Log, tc.HasLen, 1)
	c.Check(unitTask.Log[0].Message, tc.Equals, "unit log message")

	// Verify machine task
	c.Assert(op.Machines, tc.HasLen, 1)
	machineTask := op.Machines[0]
	c.Check(machineTask.ID, tc.Equals, "machine-task")
	c.Check(machineTask.ActionName, tc.Equals, "test-action")
	c.Check(machineTask.Status, tc.Equals, corestatus.Completed)
	c.Check(machineTask.ReceiverName, tc.Equals, coremachine.Name("0"))
	c.Check(machineTask.Parameters["machine-param"], tc.Equals, "machine-value")
	c.Assert(machineTask.Log, tc.HasLen, 1)
	c.Check(machineTask.Log[0].Message, tc.Equals, "machine log message")
}

func (s *querySuite) TestGetOperationsNoMatchingFilters(c *tc.C) {
	// Arrange - create operations but filter for non-existent data
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	opUUID := s.addOperation(c)
	s.addOperationAction(c, opUUID, charmUUID, "test-action")
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")
	taskUUID := s.addOperationTask(c, opUUID)
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	testCases := []struct {
		name   string
		filter operation.QueryArgs
	}{
		{
			name: "non-existent action",
			filter: operation.QueryArgs{
				ActionNames: []string{"non-existent-action"},
			},
		},
		{
			name: "non-existent unit",
			filter: operation.QueryArgs{
				Units: []coreunit.Name{"non-existent/0"},
			},
		},
		{
			name: "non-existent machine",
			filter: operation.QueryArgs{
				Machines: []coremachine.Name{"999"},
			},
		},
		{
			name: "non-existent application",
			filter: operation.QueryArgs{
				Applications: []string{"non-existent-app"},
			},
		},
		{
			name: "non-existent status",
			filter: operation.QueryArgs{
				Status: []corestatus.Status{corestatus.Error},
			},
		},
	}

	for _, testCase := range testCases {
		c.Logf("Testing filter: %s", testCase.name)
		result, err := s.state.GetOperations(c.Context(), testCase.filter)
		c.Assert(err, tc.IsNil)
		c.Assert(result.Operations, tc.HasLen, 0, tc.Commentf("Filter: %s", testCase.name))
		c.Check(result.Truncated, tc.Equals, false, tc.Commentf("Filter: %s", testCase.name))
	}
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
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")

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
