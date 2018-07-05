// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
)

// StoreOperationSuite verifies behaviour when claiming, extending, and expiring leases.
type StoreOperationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&StoreOperationSuite{})

func (s *StoreOperationSuite) TestClaimLease(c *gc.C) {
	fix := s.EasyFixture(c)

	leaseDuration := time.Minute
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed, for an exact duration.
	c.Check(key("name"), fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check(key("name"), fix.Expiry(), exactExpiry)
}

func (s *StoreOperationSuite) TestClaimMultipleLeases(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("short"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Store.ClaimLease(key("medium"), lease.Request{"grasper", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Store.ClaimLease(key("long"), lease.Request{"clutcher", time.Hour})
	c.Assert(err, jc.ErrorIsNil)

	check := func(name, holder string, duration time.Duration) {
		c.Check(key(name), fix.Holder(), holder)
		expiry := fix.Zero.Add(duration)
		c.Check(key(name), fix.Expiry(), expiry)
	}
	check("short", "holder", time.Second)
	check("medium", "grasper", time.Minute)
	check("long", "clutcher", time.Hour)
}

func (s *StoreOperationSuite) TestCannotClaimLeaseTwice(c *gc.C) {
	fix := s.EasyFixture(c)

	leaseDuration := time.Minute
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed and cannot be claimed again...
	err = fix.Store.ClaimLease(key("name"), lease.Request{"other-holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even for the same holder...
	err = fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even when the lease has expired.
	fix.GlobalClock.Advance(time.Hour)
	err = fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *StoreOperationSuite) TestExtendLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Store.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended, *to* (not by) the exact duration requested.
	c.Check(key("name"), fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check(key("name"), fix.Expiry(), exactExpiry)
}

func (s *StoreOperationSuite) TestCanExtendStaleLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// Advance the clock past lease expiry time, then extend.
	fix.LocalClock.Advance(time.Minute)
	extendTime := fix.LocalClock.Now()
	leaseDuration := time.Minute
	err = fix.Store.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended fine, *to* (not by) the exact duration requested.
	c.Check(key("name"), fix.Holder(), "holder")
	exactExpiry := extendTime.Add(leaseDuration)
	c.Check(key("name"), fix.Expiry(), exactExpiry)
}

func (s *StoreOperationSuite) TestExtendLeaseCannotChangeHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Store.ExtendLease(key("name"), lease.Request{"other-holder", leaseDuration})
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *StoreOperationSuite) TestExtendLeaseCannotShortenLease(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// A non-extension will succeed -- we can still honour all guarantees
	// implied by a nil error...
	err = fix.Store.ExtendLease(key("name"), lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// ...but we can't make it any shorter, lest we fail to honour the
	// guarantees implied by the original lease.
	c.Check(key("name"), fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check(key("name"), fix.Expiry(), exactExpiry)
}

func (s *StoreOperationSuite) TestCannotExpireLeaseBeforeExpiry(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can't be expired until after the duration has elapsed.
	fix.GlobalClock.Advance(leaseDuration)
	err = fix.Store.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *StoreOperationSuite) TestExpireLeaseAfterExpiry(c *gc.C) {
	fix := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix.Store.ClaimLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can be expired as soon as the duration has elapsed
	// *on the global clock*. The amount of time elapsed on
	// the local clock is inconsequential.
	fix.LocalClock.Advance(leaseDuration + time.Nanosecond)
	err = fix.Store.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)

	fix.GlobalClock.Advance(leaseDuration + time.Nanosecond)
	err = fix.Store.ExpireLease(key("name"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key("name"), fix.Holder(), "")
}

func (s *StoreOperationSuite) TestCannotExpireUnheldLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ExpireLease(key("name"))
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}
