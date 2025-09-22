// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
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

func (s *querySuite) TestGetOperationByID_UnitAndMachineTasks(c *tc.C) {
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

func (s *querySuite) TestGetOperationByID_NotFound(c *tc.C) {
	// Act
	_, err := s.state.GetOperationByID(c.Context(), "non-existent-id")

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.OperationNotFound)
	c.Assert(err, tc.ErrorMatches, `operation "non-existent-id" not found`)
}
