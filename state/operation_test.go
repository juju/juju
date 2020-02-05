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

	operationId, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)

	operation, err := s.Model.Operation(operationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Id(), gc.Equals, operationId)
	c.Assert(operation.Tag(), gc.Equals, names.NewOperationTag(operationId))
	c.Assert(operation.Status(), gc.Equals, state.ActionPending)
	c.Assert(operation.Enqueued(), gc.Equals, clock.Now())
	c.Assert(operation.Started(), gc.Equals, time.Time{})
	c.Assert(operation.Completed(), gc.Equals, time.Time{})
	c.Assert(operation.Summary(), gc.Equals, "an operation")
}

func (s *OperationSuite) TestAllOperations(c *gc.C) {
	operationId, err := s.Model.EnqueueOperation("an operation")
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
	c.Assert(ids, jc.SameContents, []string{operationId, operationId2})
}

func (s *OperationSuite) TestOperationStatus(c *gc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	operationId, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	clock.Advance(5 * time.Second)
	anAction, err := s.Model.EnqueueAction(operationId, unit.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	operation, err := s.Model.Operation(operationId)
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

	operationId, err := s.Model.EnqueueOperation("an operation")
	c.Assert(err, jc.ErrorIsNil)
	operation, err := s.Model.Operation(operationId)
	c.Assert(err, jc.ErrorIsNil)

	anAction, err := s.Model.EnqueueAction(operationId, unit.Tag(), "backup", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	err = operation.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Status(), gc.Equals, state.ActionRunning)
}
