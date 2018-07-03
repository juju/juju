// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
)

// ClientRemoteSuite checks that clients do not break one another's promises.
type ClientRemoteSuite struct {
	FixtureSuite
	lease        time.Duration
	localOffset  time.Duration
	globalOffset time.Duration
	baseline     *Fixture
	skewed       *Fixture
}

var _ = gc.Suite(&ClientRemoteSuite{})

func (s *ClientRemoteSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)

	s.lease = time.Minute

	// the skewed client's clock is 3 hours ahead of the local client's.
	s.localOffset = 3 * time.Hour

	// the skewed client takes an additional second to observe the global
	// time advance; when the local client thinks the global time is T,
	// the remote client thinks it is T-1s.
	s.globalOffset = -time.Second

	s.baseline = s.EasyFixture(c)
	err := s.baseline.Client.ClaimLease(key("name"), lease.Request{"holder", s.lease})
	c.Assert(err, jc.ErrorIsNil)

	// Remote client, whose local clock is offset significantly from the
	// local client's, but has a slightly delayed global clock.
	s.skewed = s.NewFixture(c, FixtureParams{
		Id:                "remote-client",
		LocalClockStart:   s.baseline.Zero.Add(s.localOffset),
		GlobalClockOffset: s.globalOffset,
	})
}

func (s *ClientRemoteSuite) skewedExpiry() time.Time {
	return s.baseline.Zero.Add(s.lease + s.localOffset - s.globalOffset)
}

// TestExpiryLocalOffset shows that the expiry time reported for the lease is
// offset by the local clock of the client, and the client's observation of
// the global clock.
func (s *ClientRemoteSuite) TestExpiryOffset(c *gc.C) {
	c.Check("name", s.skewed.Holder(), "holder")
	c.Check("name", s.skewed.Expiry(), s.skewedExpiry())
}

func (s *ClientRemoteSuite) TestExtendRemoteLeaseNoop(c *gc.C) {
	err := s.skewed.Client.ExtendLease(key("name"), lease.Request{"holder", 10 * time.Second})
	c.Check(err, jc.ErrorIsNil)

	c.Check("name", s.skewed.Holder(), "holder")
	c.Check("name", s.skewed.Expiry(), s.skewedExpiry())
}

func (s *ClientRemoteSuite) TestExtendRemoteLeaseSimpleExtend(c *gc.C) {
	leaseDuration := 10 * time.Minute
	err := s.skewed.Client.ExtendLease(key("name"), lease.Request{"holder", leaseDuration})
	c.Check(err, jc.ErrorIsNil)

	c.Check("name", s.skewed.Holder(), "holder")
	expectExpiry := s.skewed.LocalClock.Now().Add(leaseDuration)
	c.Check("name", s.skewed.Expiry(), expectExpiry)
}

func (s *ClientRemoteSuite) TestCannotExpireRemoteLeaseEarly(c *gc.C) {
	s.skewed.LocalClock.Reset(s.skewedExpiry())
	err := s.skewed.Client.ExpireLease(key("name"))
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientRemoteSuite) TestCanExpireRemoteLease(c *gc.C) {
	s.skewed.GlobalClock.Reset(s.skewedExpiry().Add(time.Nanosecond))
	err := s.skewed.Client.ExpireLease(key("name"))
	c.Check(err, jc.ErrorIsNil)
}

// ------------------------------------
