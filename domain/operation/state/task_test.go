// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
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
	c.Assert(task.Parameters, tc.HasLen, 2)
	c.Check(task.Parameters["param1"], tc.Equals, "value1")
	c.Check(task.Parameters["param2"], tc.Equals, "value2")
}

func (s *taskSuite) TestGetTaskWithTypedParameters(c *tc.C) {
	// Arrange: create an operation and a task, add parameters that look like different types
	taskID := "43"
	operationUUID := s.addOperation(c)
	s.addOperationTaskWithID(c, operationUUID, taskID, "running")
	// Values that should be decoded to specific types by encodeTask
	s.addOperationParameter(c, operationUUID, "int", "42")
	s.addOperationParameter(c, operationUUID, "neg-int", "-7")
	s.addOperationParameter(c, operationUUID, "float", "3.14")
	s.addOperationParameter(c, operationUUID, "bool-true", "true")
	s.addOperationParameter(c, operationUUID, "bool-false", "False")
	// Values that should remain as strings
	s.addOperationParameter(c, operationUUID, "str", "hello")
	s.addOperationParameter(c, operationUUID, "quoted-num", `"42"`)

	// Act
	task, _, err := s.state.GetTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// int64
	vInt, ok := task.Parameters["int"].(int64)
	c.Assert(ok, tc.Equals, true)
	c.Check(vInt, tc.Equals, int64(42))
	vNegInt, ok := task.Parameters["neg-int"].(int64)
	c.Assert(ok, tc.Equals, true)
	c.Check(vNegInt, tc.Equals, int64(-7))
	// float64
	vFloat, ok := task.Parameters["float"].(float64)
	c.Assert(ok, tc.Equals, true)
	c.Check(vFloat, tc.Equals, 3.14)
	// bool
	vTrue, ok := task.Parameters["bool-true"].(bool)
	c.Assert(ok, tc.Equals, true)
	c.Check(vTrue, tc.Equals, true)
	vFalse, ok := task.Parameters["bool-false"].(bool)
	c.Assert(ok, tc.Equals, true)
	c.Check(vFalse, tc.Equals, false)
	// string stays string
	vStr, ok := task.Parameters["str"].(string)
	c.Assert(ok, tc.Equals, true)
	c.Check(vStr, tc.Equals, "hello")
	vQte, ok := task.Parameters["quoted-num"].(string)
	c.Assert(ok, tc.Equals, true)
	c.Check(vQte, tc.Equals, "42")
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
	c.Assert(task.Log, tc.HasLen, 2)
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

func (s *taskSuite) TestGetReceiverFromTaskIDMachine(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)

	taskUUIDOne := s.addOperationTask(c, operationUUID)
	unitUUIDOne := s.addUnitWithName(c, s.addCharm(c), "test-app/0")
	s.addOperationUnitTask(c, taskUUIDOne, unitUUIDOne)

	taskIDTwo := "47"
	taskUUIDTwo := s.addOperationTaskWithID(c, operationUUID, taskIDTwo, "running")
	expectedReceiver := "7"
	machineUUID := s.addMachine(c, expectedReceiver)
	s.addOperationMachineTask(c, taskUUIDTwo, machineUUID)

	// Act
	receiver, err := s.state.GetReceiverFromTaskID(c.Context(), taskIDTwo)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(receiver, tc.Equals, expectedReceiver)
}

func (s *taskSuite) TestGetReceiverFromTaskIDUnit(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)

	taskUUIDOne := s.addOperationTask(c, operationUUID)
	unitUUIDOne := s.addUnitWithName(c, s.addCharm(c), "test-app/0")
	s.addOperationUnitTask(c, taskUUIDOne, unitUUIDOne)

	taskIDTwo := "47"
	taskUUIDTwo := s.addOperationTaskWithID(c, operationUUID, taskIDTwo, "running")
	expectedReceiver := "test-app-two/1"
	unitUUIDTwo := s.addUnitWithName(c, s.addCharm(c), expectedReceiver)
	s.addOperationUnitTask(c, taskUUIDTwo, unitUUIDTwo)

	// Act
	receiver, err := s.state.GetReceiverFromTaskID(c.Context(), taskIDTwo)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(receiver, tc.Equals, expectedReceiver)
}

func (s *taskSuite) TestGetReceiverFromTaskIDNotFound(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)

	taskUUIDOne := s.addOperationTask(c, operationUUID)
	unitUUIDOne := s.addUnitWithName(c, s.addCharm(c), "test-app/0")
	s.addOperationUnitTask(c, taskUUIDOne, unitUUIDOne)

	taskUUIDTwo := s.addOperationTask(c, operationUUID)
	unitUUIDTwo := s.addUnitWithName(c, s.addCharm(c), "test-app-two/1")
	s.addOperationUnitTask(c, taskUUIDTwo, unitUUIDTwo)

	// Act
	_, err := s.state.GetReceiverFromTaskID(c.Context(), "89")

	// Assert
	c.Assert(err, tc.ErrorIs, errors.TaskNotFound)
}

func (s *taskSuite) TestGetTaskStatusByID(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID := "42"
	s.addOperationTaskWithID(c, operationUUID, taskID, corestatus.Running.String())

	// Act
	obtainedStatus, err := s.state.GetTaskStatusByID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.Equals, corestatus.Running.String())
}

func (s *taskSuite) TestGetTaskStatusByIDNotFound(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID := "42"
	s.addOperationTask(c, operationUUID)

	// Act
	_, err := s.state.GetTaskStatusByID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIs, errors.TaskNotFound)
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

func (s *taskSuite) TestGetMachineTaskIDsWithStatusFiltersByMachineAndStatus(c *tc.C) {
	// Arrange
	m0 := s.addMachine(c, "0")
	m1 := s.addMachine(c, "1")
	op := s.addOperation(c)
	// tasks on machine 0
	t1 := s.addOperationTaskWithID(c, op, "running-id-1", corestatus.Running.String())
	s.addOperationMachineTask(c, t1, m0)
	t2 := s.addOperationTaskWithID(c, op, "running-id-2", corestatus.Running.String())
	s.addOperationMachineTask(c, t2, m0)
	t3 := s.addOperationTaskWithID(c, op, "pending-id", corestatus.Pending.String())
	s.addOperationMachineTask(c, t3, m0)
	// task on machine 1 with matching status to ensure filtering by machine
	s.addOperationMachineTask(c, s.addOperationTask(c, op), m1)

	// Act
	ids, err := s.state.GetMachineTaskIDsWithStatus(c.Context(), "0", corestatus.Running.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ids, tc.SameContents, []string{"running-id-1", "running-id-2"})
}

func (s *taskSuite) TestGetMachineTaskIDsWithStatusNoMatch(c *tc.C) {
	// Arrange
	m0 := s.addMachine(c, "0")
	op := s.addOperation(c)
	t1 := s.addOperationTaskWithID(c, op, "t1", corestatus.Pending.String())
	s.addOperationMachineTask(c, t1, m0)

	// Act
	ids, err := s.state.GetMachineTaskIDsWithStatus(c.Context(), "0", corestatus.Running.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ids, tc.HasLen, 0)
}

func (s *taskSuite) TestFinishTaskNotOperation(c *tc.C) {
	// Arrange
	// Add an operation and two tasks, neither have been completed.
	operationUUID := s.addOperation(c)
	s.addOperationTaskStatus(c, s.addOperationTask(c, operationUUID), corestatus.Running.String())
	taskUUID := s.addOperationTask(c, operationUUID)
	s.addOperationTaskStatus(c, taskUUID, corestatus.Running.String())

	// Setup the object store data to save
	storePath := "store/path"
	s.linkMetadataStorePath(c, s.addFakeMetadataStore(c, 4), storePath)

	arg := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StorePath: storePath,
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

func (s *taskSuite) TestFinishTaskNotOperationNoStoredOutput(c *tc.C) {
	// Arrange
	// Add an operation and two tasks, neither have been completed.
	operationUUID := s.addOperation(c)
	s.addOperationTaskStatus(c, s.addOperationTask(c, operationUUID), corestatus.Aborting.String())
	taskUUID := s.addOperationTask(c, operationUUID)
	s.addOperationTaskStatus(c, taskUUID, corestatus.Aborting.String())

	arg := internal.CompletedTask{
		TaskUUID: taskUUID,
		Status:   corestatus.Aborted.String(),
		Message:  "done",
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
	// Use a known task ID so we can retrieve it later and validate the message.
	taskID := "finish-op-task"
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, corestatus.Running.String())

	// Setup the object store data to save
	storePath := "store/path"
	s.linkMetadataStorePath(c, s.addFakeMetadataStore(c, 4), storePath)

	arg := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StorePath: storePath,
		Status:    corestatus.Completed.String(),
		Message:   "done",
	}

	// Act
	err := s.state.FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkTaskStatus(c, taskUUID, arg.Status)
	s.checkOperationCompleted(c, operationUUID, true)

	// Also assert that the task message is correctly returned when retrieving the task.
	task, _, err := s.state.GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(task.Message, tc.Equals, "done")
}

func (s *taskSuite) TestLogTaskMessage(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID := "42"
	taskUUID := s.addOperationTaskWithID(c, operationUUID, taskID, corestatus.Running.String())
	taskMsg := "log message"

	// Act
	err := s.state.LogTaskMessage(c.Context(), taskID, taskMsg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	var obtainedTaskMsg string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT content
FROM   operation_task_log
WHERE  task_uuid = ?
`, taskUUID).Scan(&obtainedTaskMsg)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(obtainedTaskMsg, tc.Equals, taskMsg)
}

// TestGetOperationTasksWithSameStoreMetadata verifies that tasks are not
// duplicated on fetch when they share the same store metadata.
func (s *taskSuite) TestGetOperationTasksWithSameStoreMetadata(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	taskID1 := "42"
	taskID2 := "84"
	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, taskID1, corestatus.Completed.String())
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, taskID2, corestatus.Completed.String())
	storeUUID := s.addFakeMetadataStore(c, 4)
	// the store metadata is shared by both tasks, as if they output the same datas
	s.linkMetadataStorePath(c, storeUUID, taskUUID1)
	s.linkMetadataStorePath(c, storeUUID, taskUUID2)
	s.linkOperationTaskOutput(c, taskUUID1, taskUUID1)
	s.linkOperationTaskOutput(c, taskUUID2, taskUUID2)

	// Act
	var tasks map[string][]taskResult
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) (err error) {
		tasks, err = s.state.getOperationTasks(ctx, tx, []string{operationUUID})
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(slices.Collect(maps.Keys(tasks)), tc.SameContents, []string{operationUUID})
	taskIDs := transform.Slice(tasks[operationUUID], func(f taskResult) string {
		return f.TaskID
	})
	c.Check(taskIDs, tc.SameContents, []string{taskID1, taskID2})

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
