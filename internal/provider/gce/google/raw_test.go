// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
)

type rawConnSuite struct {
	BaseSuite

	op      *compute.Operation
	rawConn *rawConn

	strategy retry.CallArgs

	handleOperationErrorsF handleOperationErrors
	callCount              int
	opCallErr              error
}

var _ = tc.Suite(&rawConnSuite{})

func (s *rawConnSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.op = &compute.Operation{
		Name:   "some_op",
		Status: StatusDone,
	}
	service := &compute.Service{}
	service.ZoneOperations = compute.NewZoneOperationsService(service)
	service.RegionOperations = compute.NewRegionOperationsService(service)
	service.GlobalOperations = compute.NewGlobalOperationsService(service)
	s.rawConn = &rawConn{service}
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

func (s *rawConnSuite) TestConnectionCheckOperationError(c *tc.C) {
	s.opCallErr = errors.New("<unknown>")

	_, err := s.rawConn.checkOperation("proj", s.op)

	c.Check(err, tc.ErrorMatches, ".*<unknown>")
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationZone(c *tc.C) {
	original := &compute.Operation{Zone: "a-zone"}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, tc.NotNil)
	c.Check(op, tc.Not(tc.Equals), original)
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationRegion(c *tc.C) {
	original := &compute.Operation{Region: "a"}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, tc.NotNil)
	c.Check(op, tc.Not(tc.Equals), original)
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionCheckOperationGlobal(c *tc.C) {
	original := &compute.Operation{}
	op, err := s.rawConn.checkOperation("proj", original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, tc.NotNil)
	c.Check(op, tc.Not(tc.Equals), original)
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperation(c *tc.C) {
	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, jc.ErrorIsNil)
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperationAlreadyDone(c *tc.C) {
	original := &compute.Operation{
		Status: StatusDone,
	}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, jc.ErrorIsNil)
	c.Check(s.callCount, tc.Equals, 0)
}

func (s *rawConnSuite) TestConnectionWaitOperationWaiting(c *tc.C) {
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
	c.Check(s.callCount, tc.Equals, 2)
}

func (s *rawConnSuite) TestConnectionWaitOperationTimeout(c *tc.C) {
	s.op.Status = StatusRunning
	err := s.rawConn.waitOperation("proj", s.op, s.strategy, s.handleOperationErrorsF)

	c.Check(err, tc.ErrorMatches, ".* timed out .*")
	c.Check(s.callCount, tc.Equals, 4)
}

func (s *rawConnSuite) TestConnectionWaitOperationFailure(c *tc.C) {
	s.opCallErr = errors.New("<unknown>")

	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, tc.ErrorMatches, ".*<unknown>")
	c.Check(s.callCount, tc.Equals, 1)
}

func (s *rawConnSuite) TestConnectionWaitOperationError(c *tc.C) {
	s.op.Error = &compute.OperationError{}
	s.op.Name = "testing-wait-operation-error"

	original := &compute.Operation{}
	err := s.rawConn.waitOperation("proj", original, s.strategy, s.handleOperationErrorsF)

	c.Check(err, tc.ErrorMatches, `.* "testing-wait-operation-error" .*`)
	c.Check(s.callCount, tc.Equals, 1)
}

type firewallNameSuite struct{}

var _ = tc.Suite(&firewallNameSuite{})

func (s *firewallNameSuite) TestSimplePattern(c *tc.C) {
	res := MatchesPrefix("juju-3-123", "juju-3")
	c.Assert(res, tc.Equals, true)
}

func (s *firewallNameSuite) TestExactMatch(c *tc.C) {
	res := MatchesPrefix("juju-3", "juju-3")
	c.Assert(res, tc.Equals, true)
}

func (s *firewallNameSuite) TestThatJujuMachineIDsDoNotCollide(c *tc.C) {
	res := MatchesPrefix("juju-30-123", "juju-3")
	c.Assert(res, tc.Equals, false)
}
