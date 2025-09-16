// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type deleteOperationSuite struct {
	baseSuite
}

func TestDeleteOperationSuite(t *testing.T) {
	tc.Run(t, &deleteOperationSuite{})
}

// TestDeleteOperationByUUIDs tests that the delete operation by UUIDs function
// deletes the operations and dependent tasks.
func (s *deleteOperationSuite) TestDeleteOperationByUUIDs(c *tc.C) {
	// Arrange: three operations in the database, two should be deleted.
	toDeleteOp1 := s.addOperation(c)
	toDeleteOp2 := s.addOperation(c)
	controlOp := s.addOperation(c)
	s.addOperationAction(c, toDeleteOp2, s.addCharm(c), "todelete")
	s.addOperationAction(c, controlOp, s.addCharm(c), "control")
	s.addOperationParameter(c, toDeleteOp2, "todelete", "value1")
	s.addOperationParameter(c, controlOp, "control", "value2")

	s.addOperationTask(c, toDeleteOp1)
	s.addOperationTask(c, controlOp)

	// Act: delete the operations.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteOperationByUUIDs(ctx, tx, []string{toDeleteOp1, toDeleteOp2})
	})

	// Assert: the operations are deleted.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{controlOp})

	for _, table := range []string{
		"operation_task",
		"operation_action",
		"operation_parameter",
	} {
		c.Check(s.selectDistinctValues(c, "operation_uuid", table), tc.SameContents, []string{controlOp},
			tc.Commentf("table: %s", table))
	}

}

// TestDeleteOperationByUUIDsEmptyInput tests that the delete operation by UUIDs
// function does nothing when the input is empty.
func (s *deleteOperationSuite) TestDeleteOperationByUUIDsEmptyInput(c *tc.C) {
	// Arrange: few operations in the database
	expected := []string{
		s.addOperation(c),
		s.addOperation(c),
	}

	// Act: delete empty input.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteOperationByUUIDs(ctx, tx, []string{})
	})

	// Assert: no error and no change in operations.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, expected)
}

// TestDeleteOperationByUUIDsEmptyInput tests that the delete operation by UUIDs
// function does nothing when the input is empty.
func (s *deleteOperationSuite) TestDeleteOperationByUUIDsNilInput(c *tc.C) {
	// Arrange: few operations in the database
	expected := []string{
		s.addOperation(c),
		s.addOperation(c),
	}

	// Act: delete empty input.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteOperationByUUIDs(ctx, tx, nil)
	})

	// Assert: no error and no change in operations.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, expected)
}

// TestDeleteTaskByUUIDs tests that the delete task by UUIDs function deletes
// the tasks and their associated dependencies.
func (s *deleteOperationSuite) TestDeleteTaskByUUIDs(c *tc.C) {
	// Arrange: three tasks in the database in various ops, two should be deleted
	operation1 := s.addOperation(c)
	operation2 := s.addOperation(c)
	toDeleteTask1 := s.addOperationTask(c, operation1)
	toDeleteTask2 := s.addOperationTask(c, operation2)
	controlTask1 := s.addOperationTask(c, operation1)
	controlTask2 := s.addOperationTask(c, operation1)
	s.addOperationUnitTask(c, toDeleteTask2, s.addUnit(c, s.addCharm(c)))
	s.addOperationUnitTask(c, controlTask1, s.addUnit(c, s.addCharm(c)))
	s.addOperationMachineTask(c, toDeleteTask1, s.addMachine(c, "machine1"))
	s.addOperationMachineTask(c, controlTask2, s.addMachine(c, "machine2"))
	s.addOperationTaskOutput(c, toDeleteTask1)
	s.addOperationTaskOutput(c, controlTask1)
	s.addOperationTaskOutput(c, controlTask2)
	s.addOperationTaskStatus(c, toDeleteTask2, "pending")
	s.addOperationTaskStatus(c, controlTask1, "pending")
	s.addOperationTaskStatus(c, controlTask2, "pending")
	s.addOperationTaskLog(c, toDeleteTask1, "log1")
	s.addOperationTaskLog(c, controlTask1, "log2")
	s.addOperationTaskLog(c, controlTask2, "log2")

	// Act: delete the tasks.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteTaskByUUIDs(ctx, tx, []string{toDeleteTask1, toDeleteTask2})
	})

	// Assert: the tasks are deleted.
	c.Assert(err, tc.ErrorIsNil)
	// no change in operations
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{operation1,
		operation2})

	c.Check(s.selectDistinctValues(c, "uuid", "operation_task"), tc.SameContents, []string{controlTask1, controlTask2})
	c.Check(s.selectDistinctValues(c, "task_uuid", "operation_unit_task"), tc.SameContents, []string{controlTask1})
	c.Check(s.selectDistinctValues(c, "task_uuid", "operation_machine_task"), tc.SameContents, []string{controlTask2})
	for _, table := range []string{
		"operation_task_output",
		"operation_task_status",
		"operation_task_log",
	} {
		c.Check(s.selectDistinctValues(c, "task_uuid", table), tc.SameContents, []string{controlTask1, controlTask2},
			tc.Commentf("table: %s", table))
	}
}

// TestDeleteTaskByUUIDsEmptyInput tests that the delete operation by UUIDs
// function does nothing when the input is empty.
func (s *deleteOperationSuite) TestDeleteTaskByUUIDsEmptyInput(c *tc.C) {
	// Arrange: few tasks in the database
	expected := []string{
		s.addOperationTask(c, s.addOperation(c)),
		s.addOperationTask(c, s.addOperation(c)),
	}

	// Act: delete empty input.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteTaskByUUIDs(ctx, tx, []string{})
	})

	// Assert: no error and no change in operations.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation_task"), tc.SameContents, expected)
}

// TestDeleteTaskByUUIDsEmptyInput tests that the delete operation by UUIDs
// function does nothing when the input is empty.
func (s *deleteOperationSuite) TestDeleteTaskByUUIDsNilInput(c *tc.C) {
	// Arrange: few tasks in the database
	expected := []string{
		s.addOperationTask(c, s.addOperation(c)),
		s.addOperationTask(c, s.addOperation(c)),
	}

	// Act: delete empty input.
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.deleteTaskByUUIDs(ctx, tx, nil)
	})

	// Assert: no error and no change in operations.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation_task"), tc.SameContents, expected)
}

// TestGetTaskUUIDsByOperationUUIDs tests that the get task UUIDs by operation
// UUIDs function returns the UUIDs of the tasks associated with the operations
func (s *deleteOperationSuite) TestGetTaskUUIDsByOperationUUIDs(c *tc.C) {
	// Arrange: three tasks in the database in various ops
	operation1 := s.addOperation(c)
	operation2 := s.addOperation(c)
	controlOperation := s.addOperation(c)
	taskToGet1 := s.addOperationTask(c, operation1)
	taskToGet2 := s.addOperationTask(c, operation1)
	taskToGet3 := s.addOperationTask(c, operation2)
	controlTask := s.addOperationTask(c, controlOperation)

	// Act: get task for operation 1 & 2
	var taskUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		taskUUIDs, err = s.state.getTaskUUIDsByOperationUUIDs(ctx, tx, []string{operation1, operation2})
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation_task"), tc.SameContents, []string{taskToGet1,
		taskToGet2, taskToGet3, controlTask})
	c.Check(taskUUIDs, tc.DeepEquals, []string{taskToGet1, taskToGet2, taskToGet3})
}

// TestGetTaskUUIDsByOperationUUIDsEmptyInput tests that the function returns
// an empty list when the input is an empty slice.
func (s *deleteOperationSuite) TestGetTaskUUIDsByOperationUUIDsEmptyInput(c *tc.C) {
	// Act
	var taskUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		taskUUIDs, err = s.state.getTaskUUIDsByOperationUUIDs(ctx, tx, []string{})
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(taskUUIDs, tc.HasLen, 0)
}

// TestGetTaskUUIDsByOperationUUIDsEmptyInput tests that the function returns
// an empty list when the input is a nil slice.
func (s *deleteOperationSuite) TestGetTaskUUIDsByOperationUUIDsNilInput(c *tc.C) {
	// Act
	var taskUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		taskUUIDs, err = s.state.getTaskUUIDsByOperationUUIDs(ctx, tx, nil)
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(taskUUIDs, tc.HasLen, 0)
}

// TestCountFewItems verifies that count returns the number of rows when there are a few items in the operation table.
func (s *deleteOperationSuite) TestCountFewItems(c *tc.C) {
	// Arrange: insert a few operations
	s.addOperation(c)
	s.addOperation(c)

	// Act
	var got int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		got, err = s.state.count(ctx, tx, "operation")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, 2)
}

// TestCountNoItems verifies that count returns zero when the operation table has no rows.
func (s *deleteOperationSuite) TestCountNoItems(c *tc.C) {
	// Arrange: no operations are added

	// Act
	var got int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		got, err = s.state.count(ctx, tx, "operation")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, 0)
}

type retrieveAndFilterSuite struct {
	baseSuite
}

func TestRetrieveAndFilterSuite(t *testing.T) {
	tc.Run(t, &retrieveAndFilterSuite{})
}

func (s *retrieveAndFilterSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

func (s *retrieveAndFilterSuite) TestGetUnitUUIDByName(c *tc.C) {
	// Arrange
	unitUUID := s.addUnitWithName(c, s.addCharm(c), "test-app/0")

	// Act
	result, err := s.state.GetUnitUUIDByName(c.Context(), coreunit.Name("test-app/0"))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, unitUUID)
}

func (s *retrieveAndFilterSuite) TestGetUnitUUIDByNameNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetUnitUUIDByName(c.Context(), coreunit.Name("non-existent/0"))

	// Assert
	c.Assert(err, tc.ErrorMatches, `getting unit UUID for "non-existent/0": unit "non-existent/0" not found`)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *retrieveAndFilterSuite) TestGetMachineUUIDByName(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c, "0")

	// Act
	result, err := s.state.GetMachineUUIDByName(c.Context(), coremachine.Name("0"))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, machineUUID)
}

func (s *retrieveAndFilterSuite) TestGetMachineUUIDByNameNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetMachineUUIDByName(c.Context(), coremachine.Name("999"))

	// Assert
	c.Assert(err, tc.ErrorMatches, `getting machine UUID for "999": machine "999" not found`)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForUnit(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	unitUUID := s.addUnit(c, s.addCharm(c))

	// Add tasks with different statuses
	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "running")
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "pending")
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "completed")

	// Link tasks to unit
	s.addOperationUnitTask(c, taskUUID1, unitUUID)
	s.addOperationUnitTask(c, taskUUID2, unitUUID)
	s.addOperationUnitTask(c, taskUUID3, unitUUID)

	taskUUIDs := []string{taskUUID1, taskUUID2, taskUUID3}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), taskUUIDs, unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Because the FilterTaskUUIDsForUnit does not perform any checks on the
	// task status' (see method's documentation), this test shows that it will
	// only filter regarding to the unit's UUID.
	// In this particular case, all tasks are related to the same unit, so they
	// are all returned.
	c.Check(len(result), tc.Equals, 3)
	c.Check(result, tc.SameContents, []string{"task-1", "task-2", "task-3"})
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMultipleUnits(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	unit0UUID := s.addUnit(c, s.addCharm(c))
	unit1UUID := s.addUnit(c, s.addCharm(c))

	// Add tasks with different statuses
	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "pending")
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "pending")
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "pending")

	// Link tasks to unit
	s.addOperationUnitTask(c, taskUUID1, unit1UUID)
	s.addOperationUnitTask(c, taskUUID2, unit0UUID)
	s.addOperationUnitTask(c, taskUUID3, unit1UUID)

	taskUUIDs := []string{taskUUID1, taskUUID2, taskUUID3}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), taskUUIDs, unit0UUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Only task-2 is related to unit0UUID.
	c.Check(len(result), tc.Equals, 1)
	c.Check(result, tc.SameContents, []string{"task-2"})
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForUnitEmptyList(c *tc.C) {
	// Arrange
	unitUUID := s.addUnit(c, s.addCharm(c))

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), []string{}, unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForUnitNoMatchingTasks(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	unitUUID := s.addUnit(c, s.addCharm(c))

	// Add task but don't link to unit
	taskUUID := s.addOperationTaskWithID(c, operationUUID, "task-1", "running")

	taskUUIDs := []string{taskUUID}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), taskUUIDs, unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForUnitNonExistentUUIDs(c *tc.C) {
	// Arrange
	unitUUID := s.addUnit(c, s.addCharm(c))
	nonExistentUUIDs := []string{internaluuid.MustNewUUID().String(), internaluuid.MustNewUUID().String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), nonExistentUUIDs, unitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMachine(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	machineUUID := s.addMachine(c, "0")

	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "running")
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "pending")
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "aborting")

	// Link tasks to machine.
	s.addOperationMachineTask(c, taskUUID1, machineUUID)
	s.addOperationMachineTask(c, taskUUID2, machineUUID)
	s.addOperationMachineTask(c, taskUUID3, machineUUID)

	taskUUIDs := []string{taskUUID1, taskUUID2, taskUUID3}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), taskUUIDs, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Because the FilterTaskUUIDsForMachine does not perform any checks on the
	// task status' (see method's documentation), this test shows that it will
	// only filter regarding to the machine's UUID.
	// In this particular case, all tasks are related to the same machine, so
	// they are all returned.
	c.Check(len(result), tc.Equals, 3)
	c.Check(result, tc.SameContents, []string{"task-1", "task-2", "task-3"})
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMultipleMachines(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	machine0UUID := s.addMachine(c, "0")
	machine1UUID := s.addMachine(c, "1")

	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "running")
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "pending")
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "aborting")

	// Link tasks to machine.
	s.addOperationMachineTask(c, taskUUID1, machine1UUID)
	s.addOperationMachineTask(c, taskUUID2, machine0UUID)
	s.addOperationMachineTask(c, taskUUID3, machine1UUID)

	taskUUIDs := []string{taskUUID1, taskUUID2, taskUUID3}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), taskUUIDs, machine0UUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Only task-2 is related to machine0UUID.
	c.Check(len(result), tc.Equals, 1)
	c.Check(result, tc.SameContents, []string{"task-2"})
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMachineEmptyList(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c, "0")

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), []string{}, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMachineNoMatchingTasks(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	machineUUID := s.addMachine(c, "0")

	// Add task but don't link to machine
	taskUUID := s.addOperationTaskWithID(c, operationUUID, "task-1", "running")

	taskUUIDs := []string{taskUUID}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), taskUUIDs, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *retrieveAndFilterSuite) TestFilterTaskUUIDsForMachineNonExistentUUIDs(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c, "0")
	nonExistentUUIDs := []string{internaluuid.MustNewUUID().String(), internaluuid.MustNewUUID().String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), nonExistentUUIDs, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}
