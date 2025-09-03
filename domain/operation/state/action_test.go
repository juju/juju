// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/uuid"
)

type actionSuite struct {
	baseSuite
}

func TestActionSuite(t *testing.T) {
	tc.Run(t, &actionSuite{})
}

func (s *actionSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

func (s *actionSuite) TestGetActionNotFound(c *tc.C) {
	taskID := "42"

	_, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.ErrorMatches, `getting action \"42\": action with task ID \"42\" not found`)
}

func (s *actionSuite) TestGetActionWithParameters(c *tc.C) {
	operationUUID := uuid.MustNewUUID()
	taskUUID := uuid.MustNewUUID()
	unitUUID := uuid.MustNewUUID()
	charmUUID := uuid.MustNewUUID()
	taskID := "42"

	s.insertCharm(c, charmUUID.String(), "test-charm")
	s.insertCharmAction(c, charmUUID.String(), "test-action", "Test action")
	s.insertOperation(c, operationUUID.String())
	s.insertOperationAction(c, operationUUID.String(), charmUUID.String(), "test-action")
	s.insertOperationTaskWithID(c, taskUUID.String(), operationUUID.String(), taskID)
	s.insertOperationParameter(c, operationUUID.String(), "param1", "value1")
	s.insertOperationParameter(c, operationUUID.String(), "param2", "value2")
	s.insertUnit(c, unitUUID.String(), "test-app-1/0")
	s.insertOperationUnitTask(c, taskUUID.String(), unitUUID.String())

	action, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, operationUUID)
	c.Check(action.Receiver, tc.Equals, "test-app-1/0")
	c.Check(action.Name, tc.Equals, "test-operation")
	c.Check(action.Parameters, tc.DeepEquals, map[string]any{
		"param1": "value1",
		"param2": "value2",
	})
}

func (s *actionSuite) TestGetActionWithLogs(c *tc.C) {
	operationUUID := uuid.MustNewUUID()
	taskUUID := uuid.MustNewUUID()
	unitUUID := uuid.MustNewUUID()
	charmUUID := uuid.MustNewUUID()
	taskID := "42"

	s.insertCharm(c, charmUUID.String(), "test-charm")
	s.insertCharmAction(c, charmUUID.String(), "test-action", "Test action")
	s.insertOperation(c, operationUUID.String())
	s.insertOperationAction(c, operationUUID.String(), charmUUID.String(), "test-action")
	s.insertOperationTaskWithID(c, taskUUID.String(), operationUUID.String(), taskID)
	s.insertUnit(c, unitUUID.String(), "test-app-1/0")
	s.insertOperationUnitTask(c, taskUUID.String(), unitUUID.String())
	s.insertOperationLog(c, taskUUID.String(), "log entry 1")
	s.insertOperationLog(c, taskUUID.String(), "log entry 2")

	action, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, operationUUID)
	c.Check(action.Receiver, tc.Equals, "test-app-1/0")
	c.Check(action.Name, tc.Equals, "test-operation")
	c.Check(action.Log[0].Message, tc.DeepEquals, "log entry 1")
	c.Check(action.Log[1].Message, tc.DeepEquals, "log entry 2")
}

func (s *actionSuite) TestGetActionWithUnitReceiver(c *tc.C) {
	operationUUID := uuid.MustNewUUID()
	taskUUID := uuid.MustNewUUID()
	unitUUID := uuid.MustNewUUID()
	charmUUID := uuid.MustNewUUID()
	taskID := "42"

	s.insertCharm(c, charmUUID.String(), "test-charm")
	s.insertCharmAction(c, charmUUID.String(), "test-action", "Test action")
	s.insertOperation(c, operationUUID.String())
	s.insertOperationAction(c, operationUUID.String(), charmUUID.String(), "test-action")
	s.insertOperationTaskWithID(c, taskUUID.String(), operationUUID.String(), taskID)
	s.insertUnit(c, unitUUID.String(), "test-app-1/0")
	s.insertOperationUnitTask(c, taskUUID.String(), unitUUID.String())

	action, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, operationUUID)
	c.Check(action.Receiver, tc.Equals, "test-app-1/0")
	c.Check(action.Name, tc.Equals, "test-operation")
}

func (s *actionSuite) TestGetActionWithMachineReceiver(c *tc.C) {
	operationUUID := uuid.MustNewUUID()
	taskUUID := uuid.MustNewUUID()
	machineUUID := uuid.MustNewUUID()
	charmUUID := uuid.MustNewUUID()
	taskID := "42"

	s.insertCharm(c, charmUUID.String(), "test-charm")
	s.insertCharmAction(c, charmUUID.String(), "test-action", "Test action")
	s.insertOperation(c, operationUUID.String())
	s.insertOperationAction(c, operationUUID.String(), charmUUID.String(), "test-action")
	s.insertOperationTaskWithID(c, taskUUID.String(), operationUUID.String(), taskID)
	s.insertMachine(c, machineUUID.String(), "0")
	s.insertOperationMachineTask(c, taskUUID.String(), machineUUID.String())

	action, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, operationUUID)
	c.Check(action.Receiver, tc.Equals, "0")
	c.Check(action.Name, tc.Equals, "test-operation")
}

func (s *actionSuite) TestGetActionWithoutReceiver(c *tc.C) {
	operationUUID := uuid.MustNewUUID()
	taskUUID := uuid.MustNewUUID()
	charmUUID := uuid.MustNewUUID()
	taskID := "42"

	s.insertCharm(c, charmUUID.String(), "test-charm")
	s.insertCharmAction(c, charmUUID.String(), "test-action", "Test action")
	s.insertOperation(c, operationUUID.String())
	s.insertOperationAction(c, operationUUID.String(), charmUUID.String(), "test-action")
	s.insertOperationTaskWithID(c, taskUUID.String(), operationUUID.String(), taskID)

	action, err := s.state.GetAction(context.Background(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, operationUUID)
	c.Check(action.Receiver, tc.Equals, "")
	c.Check(action.Name, tc.Equals, "test-operation")
}

func (s *actionSuite) TestCancelActionNotFound(c *tc.C) {
	taskID := "42"

	_, err := s.state.CancelAction(context.Background(), taskID)
	c.Assert(err, tc.ErrorMatches, `.*task with ID \"42\" not found`)
}
