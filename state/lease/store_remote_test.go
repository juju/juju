// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
)

// StoreRemoteSuite checks that stores do not break one another's promises.
type StoreRemoteSuite struct {
	FixtureSuite
	lease        time.Duration
	localOffset  time.Duration
	globalOffset time.Duration
	baseline     *Fixture
	skewed       *Fixture
}

var _ = gc.Suite(&StoreRemoteSuite{})

func (s *StoreRemoteSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)

	s.lease = time.Minute

	// the skewed store's clock is 3 hours ahead of the local store's.
	s.localOffset = 3 * time.Hour

	// the skewed store takes an additional second to observe the global
	// time advance; when the local store thinks the global time is T,
	// the remote store thinks it is T-1s.
	s.globalOffset = -time.Second

	s.baseline = s.EasyFixture(c)
	err := s.baseline.Store.ClaimLease(key("name"), lease.Request{"holder", s.lease})
	c.Assert(err, jc.ErrorIsNil)

	// Remote store, whose local clock is offset significantly from the
	// local store's, but has a slightly delayed global clock.
	s.skewed = s.NewFixture(c, FixtureParams{
		Id:                "remote-store",
		LocalClockStart:   s.baseline.Zero.Add(s.localOffset),
		GlobalClockOffset: s.globalOffset,
	})
}

func (s *StoreRemoteSuite) skewedExpiry() time.Time {
	return s.baseline.Zero.Add(s.lease + s.localOffset - s.globalOffset)
}

// TestExpiryLocalOffset shows that the expiry time reported for the lease is
// offset by the local clock of the store, and the store's observation of
// the global clock.
func (s *StoreRemoteSuite) TestExpiryOffset(c *gc.C) {
	c.Check(key("name"), s.skewed.Holder(), "holder")
	c.Check(key("name"), s.skewed.Expiry(), s.skewedExpiry())
}

func (s *StoreRemoteSuite) TestExtendRemoteLeaseNoop(c *gc.C) {
	err := s.skewed.Store.ExtendLease(key("name"), lease.Request{"holder", 10 * time.Second})
	c.Check(err, jc.ErrorIsNil)

	c.Check(key("name"), s.skewed.Holder(), "holder")
	c.Check(key("name"), s.skewed.Expiry(), s.skewedExpiry())
}

func (s *StoreRemoteSuite) TestExtendRemoteLeaseSimpleExtend(c *gc.C) {
	leaseDuration := 10 * time.Minute
	err := s.skewed.Store.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, jc.ErrorIsNil)

	c.Check(key("name"), s.skewed.Holder(), "holder")
	expectExpiry := s.skewed.LocalClock.Now().Add(leaseDuration)
	c.Check(key("name"), s.skewed.Expiry(), expectExpiry)
}

func (s *StoreRemoteSuite) TestCannotExpireRemoteLeaseEarly(c *gc.C) {
	s.skewed.LocalClock.Reset(s.skewedExpiry())
	err := s.skewed.Store.ExpireLease(key("name"))
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *StoreRemoteSuite) TestCanExpireRemoteLease(c *gc.C) {
	s.skewed.GlobalClock.Reset(s.skewedExpiry().Add(time.Nanosecond))
	err := s.skewed.Store.ExpireLease(key("name"))
	c.Check(err, jc.ErrorIsNil)
}

// ------------------------------------
