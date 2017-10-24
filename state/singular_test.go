// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

type SingularSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SingularSuite{})

func (s *SingularSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	err := s.State.SetClockForTesting(s.Clock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SingularSuite) TestClaimBadLease(c *gc.C) {
	claimer := s.State.SingularClaimer()
	err := claimer.Claim("xxx", "machine-123", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease "xxx": expected environ UUID`)
}

func (s *SingularSuite) TestClaimBadHolder(c *gc.C) {
	claimer := s.State.SingularClaimer()
	err := claimer.Claim(s.modelTag.Id(), "unit-foo-1", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for holder "unit-foo-1": expected machine tag`)
}

func (s *SingularSuite) TestClaimBadDuration(c *gc.C) {
	claimer := s.State.SingularClaimer()
	err := claimer.Claim(s.modelTag.Id(), "machine-123", 0)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for 0s?: non-positive`)
}

func (s *SingularSuite) TestClaim(c *gc.C) {
	claimer := s.State.SingularClaimer()
	err := claimer.Claim(s.modelTag.Id(), "machine-123", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	err = claimer.Claim(s.modelTag.Id(), "machine-123", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	err = claimer.Claim(s.modelTag.Id(), "machine-456", time.Minute)
	c.Assert(err, gc.Equals, lease.ErrClaimDenied)
}

func (s *SingularSuite) TestExpire(c *gc.C) {
	claimer := s.State.SingularClaimer()
	err := claimer.Claim(s.modelTag.Id(), "machine-123", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	wait := make(chan error)
	go func() {
		wait <- claimer.WaitUntilExpired(s.modelTag.Id())
	}()

	g, err := s.State.GlobalClockUpdater()
	c.Assert(err, jc.ErrorIsNil)
	err = g.AddTime(coretesting.ShortWait)
	c.Assert(err, jc.ErrorIsNil)
	s.Clock.Advance(coretesting.ShortWait)
	select {
	case err := <-wait:
		c.Fatalf("expired early with %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	err = g.AddTime(time.Hour)
	c.Assert(err, jc.ErrorIsNil)
	s.Clock.Advance(time.Hour)
	select {
	case err := <-wait:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never expired")
	}

	err = claimer.Claim(s.modelTag.Id(), "machine-456", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}
