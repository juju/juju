// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
)

// ClientOperationSuite verifies behaviour when claiming, extending, and expiring leases.
type ClientOperationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientOperationSuite{})

func (s *ClientOperationSuite) TestClaimLease(c *gc.C) {
	fix := s.EasyFixture(c)

	leaseDuration := time.Minute
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed, for an exact duration.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.Expiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestClaimMultipleLeases(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Client.ClaimLease(key("short"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Client.ClaimLease(key("medium"), lease.Request{"grasper", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Client.ClaimLease(key("long"), lease.Request{"clutcher", time.Hour})
	c.Assert(err, jc.ErrorIsNil)

	check := func(name, holder string, duration time.Duration) {
		c.Check(name, fix.Holder(), holder)
		expiry := fix.Zero.Add(duration)
		c.Check(name, fix.Expiry(), expiry)
	}
	check("short", "holder", time.Second)
	check("medium", "grasper", time.Minute)
	check("long", "clutcher", time.Hour)
}

func (s *ClientOperationSuite) TestCannotClaimLeaseTwice(c *gc.C) {
	fix := s.EasyFixture(c)

	leaseDuration := time.Minute
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed and cannot be claimed again...
	err = fix.Client.ClaimLease(key("name"), lease.Request{"other-holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even for the same holder...
	err = fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even when the lease has expired.
	fix.GlobalClock.Advance(time.Hour)
	err = fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExtendLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Client.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended, *to* (not by) the exact duration requested.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.Expiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestCanExtendStaleLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// Advance the clock past lease expiry time, then extend.
	fix.LocalClock.Advance(time.Minute)
	extendTime := fix.LocalClock.Now()
	leaseDuration := time.Minute
	err = fix.Client.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended fine, *to* (not by) the exact duration requested.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := extendTime.Add(leaseDuration)
	c.Check("name", fix.Expiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestExtendLeaseCannotChangeHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Client.ExtendLease(key("name"), lease.Request{"other-holder", leaseDuration})
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExtendLeaseCannotShortenLease(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// A non-extension will succeed -- we can still honour all guarantees
	// implied by a nil error...
	err = fix.Client.ExtendLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// ...but we can't make it any shorter, lest we fail to honour the
	// guarantees implied by the original lease.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.Expiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestCannotExpireLeaseBeforeExpiry(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can't be expired until after the duration has elapsed.
	fix.GlobalClock.Advance(leaseDuration)
	err = fix.Client.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExpireLeaseAfterExpiry(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can be expired as soon as the duration has elapsed
	// *on the global clock*. The amount of time elapsed on
	// the local clock is inconsequential.
	fix.LocalClock.Advance(leaseDuration + time.Nanosecond)
	err = fix.Client.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)

	fix.GlobalClock.Advance(leaseDuration + time.Nanosecond)
	err = fix.Client.ExpireLease(key("name"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check("name", fix.Holder(), "")
}

func (s *ClientOperationSuite) TestCannotExpireUnheldLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}
