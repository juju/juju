// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type OperationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&OperationSuite{})

func (s *OperationSuite) TestEnqueueOperation(c *gc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)

	operation, err := s.Model.Operation(operationID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Id(), gc.Equals, operationID)
	c.Assert(operation.Tag(), gc.Equals, names.NewOperationTag(operationID))
	c.Assert(operation.Status(), gc.Equals, state.ActionPending)
	c.Assert(operation.Enqueued(), gc.Equals, clock.Now())
	c.Assert(operation.Started(), gc.Equals, time.Time{})
	c.Assert(operation.Completed(), gc.Equals, time.Time{})
	c.Assert(operation.Summary(), gc.Equals, "an operation")
}

func (s *OperationSuite) TestAllOperations(c *gc.C) {
	operationID, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	operationId2, err := s.Model.EnqueueOperation("another operation")
	c.Assert(err, jc.ErrorIsNil)

	operations, err := s.Model.AllOperations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 2)

	var ids []string
	for _, op := range operations {
		ids = append(ids, op.Id())
	}
	c.Assert(ids, jc.SameContents, []string{operationID, operationId2})
}

func (s *OperationSuite) TestOperationStatus(c *gc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	clock.Advance(5 * time.Second)
	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	operation, err := s.Model.Operation(operationID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Status(), gc.Equals, state.ActionRunning)
	c.Assert(operation.Started(), gc.Equals, clock.Now())
	c.Assert(operation.Completed(), gc.Equals, time.Time{})
}

func (s *OperationSuite) TestRefresh(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	operation, err := s.Model.Operation(operationID)
	c.Assert(err, jc.ErrorIsNil)

	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	err = operation.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Status(), gc.Equals, state.ActionRunning)
}

func (s *OperationSuite) setupOperations(c *gc.C) names.Tag {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	operationID2, err := s.Model.EnqueueOperation("another operation")
	c.Assert(err, jc.ErrorIsNil)

	clock.Advance(5 * time.Second)
	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)
	anAction2, err := s.Model.EnqueueAction(operationID2, unit.Tag(), "restore", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction2.Begin()
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction2.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, jc.ErrorIsNil)

	unit2, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	operationID3, err := s.Model.EnqueueOperation("yet another operation")
	c.Assert(err, jc.ErrorIsNil)
	anAction3, err := s.Model.EnqueueAction(operationID3, unit2.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction3.Begin()

	return unit.Tag()
}

func (s *OperationSuite) assertActions(c *gc.C, operations []state.OperationSummary) {
	for _, operation := range operations {
		for _, a := range operation.Actions {
			c.Assert(operation.Operation.Id(), gc.Equals, state.ActionOperationId(a))
			if a.Name() == "restore" {
				c.Assert(a.Status(), gc.Equals, state.ActionCompleted)
			} else {
				c.Assert(a.Status(), gc.Equals, state.ActionRunning)
			}
		}
	}
}

func (s *OperationSuite) TestListOperationsNoFilter(c *gc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, nil, nil, 0, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(truncated, jc.IsFalse)
	c.Assert(operations, gc.HasLen, 3)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "an operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	c.Assert(operations[1].Operation.Summary(), gc.Equals, "another operation")
	c.Assert(operations[1].Actions, gc.HasLen, 1)
	c.Assert(operations[2].Operation.Summary(), gc.Equals, "yet another operation")
	c.Assert(operations[2].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperations(c *gc.C) {
	unitTag := s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations([]string{"backup"}, []names.Tag{unitTag}, []state.ActionStatus{state.ActionRunning}, 0, 0)
	c.Assert(truncated, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "an operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByStatus(c *gc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, nil, []state.ActionStatus{state.ActionCompleted}, 0, 0)
	c.Assert(truncated, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "another operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByName(c *gc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations([]string{"restore"}, nil, nil, 0, 0)
	c.Assert(truncated, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "another operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByReceiver(c *gc.C) {
	unitTag := s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, []names.Tag{unitTag}, nil, 0, 0)
	c.Assert(truncated, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 2)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "an operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	c.Assert(operations[1].Operation.Summary(), gc.Equals, "another operation")
	c.Assert(operations[1].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsSubset(c *gc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, nil, nil, 1, 1)
	c.Assert(truncated, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operations, gc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), gc.Equals, "another operation")
	c.Assert(operations[0].Actions, gc.HasLen, 1)
	s.assertActions(c, operations)
}
