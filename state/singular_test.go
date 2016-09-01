// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SingularSuite struct {
	clock *jujutesting.Clock
	ConnSuite
}

var _ = gc.Suite(&SingularSuite{})

func (s *SingularSuite) SetUpSuite(c *gc.C) {
	s.ConnSuite.SetUpSuite(c)
	s.PatchValue(&state.GetClock, func() clock.Clock {
		return s.clock
	})
}

func (s *SingularSuite) SetUpTest(c *gc.C) {
	s.clock = jujutesting.NewClock(time.Now())
	s.ConnSuite.SetUpTest(c)
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
	select {
	case err := <-wait:
		c.Fatalf("expired early with %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	s.clock.Advance(time.Hour)
	select {
	case err := <-wait:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never expired")
	}

	err = claimer.Claim(s.modelTag.Id(), "machine-456", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}
