// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/domain/operation/internal"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type taskSuite struct {
	baseSuite
}

func TestTaskSuite(t *testing.T) {
	tc.Run(t, &taskSuite{})
}

func (s *taskSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

func (s *taskSuite) TestGetTaskNotFound(c *tc.C) {
	taskID := "42"

	_, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `getting task \"42\": task with ID \"42\" not found`)
}

func (s *taskSuite) TestGetTask(c *tc.C) {
	taskID := "42"

	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	operationUUID := s.addOperationWithExecutionGroup(c, "test-group")
	s.addOperationAction(c, operationUUID, charmUUID, "test-action")
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	unitUUID := s.addUnitWithName(c, charmUUID, "test-app/0")
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	task, outputPath, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(outputPath, tc.IsNil)
	c.Check(task.ActionName, tc.Equals, "test-action")
	c.Check(task.Receiver, tc.Equals, "test-app/0")
	c.Check(task.Status, tc.Equals, corestatus.Running)
	c.Check(task.ExecutionGroup, tc.DeepEquals, ptr("test-group"))
}

func (s *taskSuite) TestGetTaskWithOutputPath(c *tc.C) {
	taskID := "42"

	operationUUID := s.addOperation(c)
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "running")

	storePath := "task-output/test-output.json"
	s.addOperationTaskOutputWithData(c, taskUUID, "sha256hash", "sha384hash", 100, storePath)

	_, outputPath, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*outputPath, tc.Equals, storePath)
}

func (s *taskSuite) TestGetTaskWithParameters(c *tc.C) {
	internaluuid.MustNewUUID()

	taskID := "42"

	operationUUID := s.addOperation(c)
	s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	s.addOperationParameter(c, operationUUID, "param1", "value1")
	s.addOperationParameter(c, operationUUID, "param2", "value2")

	task, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(task.Parameters), tc.Equals, 2)
	c.Check(task.Parameters["param1"], tc.Equals, "value1")
	c.Check(task.Parameters["param2"], tc.Equals, "value2")
}

func (s *taskSuite) TestGetTaskWithLogs(c *tc.C) {

	taskID := "42"

	operationUUID := s.addOperation(c)
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	s.addOperationTaskLog(c, taskUUID, "log entry 1")
	s.addOperationTaskLog(c, taskUUID, "log entry 2")

	task, outputPath, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(outputPath, tc.IsNil)
	c.Check(len(task.Log), tc.Equals, 2)
	c.Check(task.Log[0].Message, tc.DeepEquals, "log entry 1")
	c.Check(task.Log[1].Message, tc.DeepEquals, "log entry 2")
}

func (s *taskSuite) TestGetTaskWithUnitReceiver(c *tc.C) {

	taskID := "42"

	operationUUID := s.addOperation(c)
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	unitUUID := s.addUnitWithName(c, s.addCharm(c), "test-app/0")
	s.addOperationUnitTask(c, taskUUID, unitUUID)

	task, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(task.Receiver, tc.Equals, "test-app/0")
}

func (s *taskSuite) TestGetTaskWithMachineReceiver(c *tc.C) {
	taskID := "42"

	operationUUID := s.addOperation(c)
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	machineUUID := s.addMachine(c, "0")
	s.addOperationMachineTask(c, taskUUID, machineUUID)

	task, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(task.Receiver, tc.Equals, "0")
}

func (s *taskSuite) TestGetTaskWithoutReceiver(c *tc.C) {
	taskID := "42"

	operationUUID := s.addOperation(c)
	s.addOperationTaskWithID(c, operationUUID, taskID, "running")

	task, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(task.Receiver, tc.Equals, "")
}

func (s *taskSuite) TestCancelTaskNotFound(c *tc.C) {
	taskID := "42"

	_, err := s.state.CancelTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `.*task with ID \"42\" not found`)
}

func (s *taskSuite) TestStartTask(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID := "42"
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, "pending")

	// Act
	err := s.state.StartTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkTaskStatus(c, taskUUID, corestatus.Running.String())
}

func (s *taskSuite) TestStartTaskNotFound(c *tc.C) {
	// Arrange
	taskID := "42"

	// Act
	err := s.state.StartTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIs, errors.TaskNotFound)
}

func (s *taskSuite) TestStartTaskNotPending(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID := "42"
	s.addOperationTaskWithID(c, operationUUID, taskID, corestatus.Running.String())

	// Act
	err := s.state.StartTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIs, errors.TaskNotPending)
}

func (s *taskSuite) TestFinishTaskNotOperation(c *tc.C) {
	// Arrange
	// Add an operation and two tasks, neither have been completed.
	operationUUID := s.addOperation(c)
	s.addOperationTaskStatus(c, s.addOperationTask(c, operationUUID), corestatus.Running.String())
	taskUUID := s.addOperationTask(c, operationUUID)
	s.addOperationTaskStatus(c, taskUUID, corestatus.Running.String())

	// Setup the object store data to save
	storeUUID := s.addFakeMetadataStore(c, 4)
	s.addMetadataStorePath(c, storeUUID, taskUUID)

	arg := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID,
		Status:    corestatus.Completed.String(),
		Message:   "done",
	}

	// Act
	err := s.state.FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkTaskStatus(c, taskUUID, arg.Status)
	s.checkOperationCompleted(c, operationUUID, false)
}

func (s *taskSuite) TestFinishTaskAndOperation(c *tc.C) {
	// Arrange
	// Add an operation and two tasks, one is finished with
	// an error state.
	operationUUID := s.addOperation(c)
	s.addOperationTaskStatus(c, s.addOperationTask(c, operationUUID), corestatus.Error.String())
	taskUUID := s.addOperationTask(c, operationUUID)
	s.addOperationTaskStatus(c, taskUUID, corestatus.Running.String())

	// Setup the object store data to save
	storeUUID := s.addFakeMetadataStore(c, 4)
	s.addMetadataStorePath(c, storeUUID, taskUUID)

	arg := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID,
		Status:    corestatus.Completed.String(),
		Message:   "done",
	}

	// Act
	err := s.state.FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkTaskStatus(c, taskUUID, arg.Status)
	s.checkOperationCompleted(c, operationUUID, true)
}

func (s *taskSuite) checkTaskStatus(c *tc.C, taskUUID, status string) {
	// Assert: Check that the task status has been set as indicated
	var task string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT ots.task_uuid
FROM   operation_task_status AS ots
JOIN   operation_task_status_value AS otsv ON ots.status_id = otsv.id
WHERE  ots.task_uuid = ?
AND    otsv.status = ?
`, taskUUID, status).Scan(&task)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(task, tc.Equals, taskUUID)
}

func (s *taskSuite) checkOperationCompleted(c *tc.C, operationUUID string, completed bool) {
	// Assert: Check if the operation completed at time has been set
	// as indicated indicated by "completed"
	var completedAt sql.NullTime
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT completed_at
FROM   operation
WHERE  uuid = ?
`, operationUUID).Scan(&completedAt)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(completedAt.Time.IsZero(), tc.Equals, !completed, tc.Commentf("expected completed at %v", completedAt))
}

func ptr[T any](v T) *T {
	return &v
}
