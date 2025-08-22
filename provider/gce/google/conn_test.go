// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type rawConnSuite struct {
	testing.BaseSuite

	op      *compute.Operation
	rawConn *Connection

	strategy retry.CallArgs

	handleOperationErrorsF handleOperationErrors
	callCount              int
	opCallErr              error
}

var _ = gc.Suite(&rawConnSuite{})

func (s *rawConnSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.op = &compute.Operation{
		Name:   "some_op",
		Status: StatusDone,
	}
	service := &compute.Service{}
	service.ZoneOperations = compute.NewZoneOperationsService(service)
	service.RegionOperations = compute.NewRegionOperationsService(service)
	service.GlobalOperations = compute.NewGlobalOperationsService(service)
	s.rawConn = &Connection{Service: service, projectID: "proj"}
	s.strategy = retry.CallArgs{
		Clock:    clock.WallClock,
		Delay:    time.Millisecond,
		Attempts: 4,
	}

	s.callCount = 0
	s.opCallErr = nil
	s.PatchValue(&doOpCall, func(call opDoer) (*compute.Operation, error) {
		s.callCount++
		return s.op, s.opCallErr
	})
	s.handleOperationErrorsF = logOperationErrors
}

func (s *rawConnSuite) TestConnectionCheckOperationError(c *gc.C) {
	s.opCallErr = errors.New("<unknown>")

	_, err := s.rawConn.checkOperation("proj", s.op)

	c.Check(err, gc.ErrorMatches, ".*<unknown>")
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationZone(c *gc.C) {
	original := &compute.Operation{Zone: "a-zone"}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationRegion(c *gc.C) {
	original := &compute.Operation{Region: "a"}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationGlobal(c *gc.C) {
	original := &compute.Operation{}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperation(c *gc.C) {
	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, jc.ErrorIsNil)
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperationAlreadyDone(c *gc.C) {
	original := &compute.Operation{
		Status: StatusDone,
	}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, jc.ErrorIsNil)
	c.Check(s.callCount, gc.Equals, 0)
}

func (s *rawConnSuite) TestConnectionWaitOperationWaiting(c *gc.C) {
	s.op.Status = StatusRunning
	s.PatchValue(&doOpCall, func(call opDoer) (*compute.Operation, error) {
		s.callCount++
		if s.callCount > 1 {
			s.op.Status = StatusDone
		}
		return s.op, s.opCallErr
	})

	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, jc.ErrorIsNil)
	c.Check(s.callCount, gc.Equals, 2)
}

func (s *rawConnSuite) TestConnectionWaitOperationTimeout(c *gc.C) {
	s.op.Status = StatusRunning
	err := s.rawConn.waitOperation("proj", s.op, s.strategy, s.handleOperationErrorsF)

	c.Check(err, gc.ErrorMatches, ".* timed out .*")
	c.Check(s.callCount, gc.Equals, 4)
}

func (s *rawConnSuite) TestConnectionWaitOperationFailure(c *gc.C) {
	s.opCallErr = errors.New("<unknown>")

	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, gc.ErrorMatches, ".*<unknown>")
	c.Check(s.callCount, gc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperationError(c *gc.C) {
	s.op.Error = &compute.OperationError{}
	s.op.Name = "testing-wait-operation-error"

	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, gc.ErrorMatches, `.* "testing-wait-operation-error" .*`)
	c.Check(s.callCount, gc.Equals, 1)
}
