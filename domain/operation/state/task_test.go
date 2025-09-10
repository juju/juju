// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type taskSuite struct {
	baseSuite
}

func TestTestSuite(t *testing.T) {
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
	internaluuid.MustNewUUID()

	internaluuid.MustNewUUID()
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

func ptr[T any](v T) *T {
	return &v
}
