// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	ConnSuite
	clock   *coretesting.Clock
	checker leadership.Checker
	claimer leadership.Claimer
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpTest(c *gc.C) {
	s.clock = coretesting.NewClock(time.Now())
	s.PatchValue(&state.GetClock, func() clock.Clock {
		return s.clock
	})
	s.ConnSuite.SetUpTest(c)

	s.checker = s.State.LeadershipChecker()
	s.claimer = s.State.LeadershipClaimer()
}

func (s *LeadershipSuite) TestClaimValidatesServiceName(c *gc.C) {
	err := s.claimer.ClaimLeadership("not/a/service", "u/0", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease "not/a/service": not a service name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimValidatesUnitName(c *gc.C) {
	err := s.claimer.ClaimLeadership("service", "not/a/unit", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for holder "not/a/unit": not a unit name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimValidateDuration(c *gc.C) {
	err := s.claimer.ClaimLeadership("service", "u/0", 0)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for 0: non-positive`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestCheckValidatesServiceName(c *gc.C) {
	token := s.checker.LeadershipCheck("not/a/service", "u/0")
	err := token.Check(nil)
	c.Check(err, gc.ErrorMatches, `cannot check lease "not/a/service": not a service name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestCheckValidatesUnitName(c *gc.C) {
	token := s.checker.LeadershipCheck("service", "not/a/unit")
	err := token.Check(nil)
	c.Check(err, gc.ErrorMatches, `cannot check holder "not/a/unit": not a unit name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestBlockValidatesServiceName(c *gc.C) {
	err := s.claimer.BlockUntilLeadershipReleased("not/a/service")
	c.Check(err, gc.ErrorMatches, `cannot wait for lease "not/a/service" expiry: not a service name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimExpire(c *gc.C) {

	// Claim on behalf of one unit.
	err := s.claimer.ClaimLeadership("service", "service/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// Claim on behalf of another.
	err = s.claimer.ClaimLeadership("service", "service/1", time.Minute)
	c.Check(err, gc.Equals, leadership.ErrClaimDenied)

	// Allow the first claim to expire.
	s.expire(c, "service")

	// Reclaim on behalf of another.
	err = s.claimer.ClaimLeadership("service", "service/1", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestCheck(c *gc.C) {

	// Create a single token for use by the whole test.
	token := s.checker.LeadershipCheck("service", "service/0")

	// Claim leadership.
	err := s.claimer.ClaimLeadership("service", "service/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// Check token reports current leadership state.
	var ops []txn.Op
	err = token.Check(&ops)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ops, gc.HasLen, 1)

	// Allow leadership to expire.
	s.expire(c, "service")

	// Check leadership still reported accurately.
	var ops2 []txn.Op
	err = token.Check(&ops2)
	c.Check(err, gc.ErrorMatches, `"service/0" is not leader of "service"`)
	c.Check(ops2, gc.IsNil)
}

func (s *LeadershipSuite) TestHackLeadershipUnblocksClaimer(c *gc.C) {
	err := s.claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	s.State.HackLeadership()
	select {
	case err := <-s.expiryChan("blah"):
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for unblock")
	}
}

func (s *LeadershipSuite) expire(c *gc.C, serviceName string) {
	s.clock.Advance(time.Hour)
	select {
	case err := <-s.expiryChan(serviceName):
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never unblocked")
	}
}

func (s *LeadershipSuite) expiryChan(serviceName string) <-chan error {
	expired := make(chan error, 1)
	go func() {
		expired <- s.claimer.BlockUntilLeadershipReleased("blah")
	}()
	return expired
}
