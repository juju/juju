// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
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
