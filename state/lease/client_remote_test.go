// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/lease"
)

// ClientRemoteSuite checks that clients do not break one another's promises.
type ClientRemoteSuite struct {
	FixtureSuite
	lease    time.Duration
	offset   time.Duration
	readTime time.Duration
	baseline *Fixture
	skewed   *Fixture
}

var _ = gc.Suite(&ClientRemoteSuite{})

func (s *ClientRemoteSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)

	s.lease = time.Minute
	s.offset = time.Second
	s.readTime = 100 * time.Millisecond

	s.baseline = s.EasyFixture(c)
	err := s.baseline.Client.ClaimLease("name", lease.Request{"holder", s.lease})
	c.Assert(err, jc.ErrorIsNil)

	// Remote client, possibly reading in the future and possibly just ahead
	// by a second, taking 100ms to read the clock doc; sees same lease with
	// suitable uncertainty.
	s.skewed = s.NewFixture(c, FixtureParams{
		Id:         "remote-client",
		ClockStart: s.baseline.Zero.Add(s.offset),
		ClockStep:  s.readTime,
	})
	// We don't really want the clock to keep going outside our control here.
	s.skewed.Clock.step = 0
}

func (s *ClientRemoteSuite) guaranteedUntil() time.Time {
	return s.baseline.Zero.Add(s.lease + s.offset)
}

func (s *ClientRemoteSuite) latestValid() time.Time {
	return s.guaranteedUntil().Add(s.readTime)
}

func (s *ClientRemoteSuite) TestReadSkew(c *gc.C) {
	c.Check("name", s.skewed.Holder(), "holder")
	c.Check("name", s.skewed.Expiry(), s.latestValid())
}

func (s *ClientRemoteSuite) TestExtendRemoteLeaseNoop(c *gc.C) {
	err := s.skewed.Client.ExtendLease("name", lease.Request{"holder", 10 * time.Second})
	c.Check(err, jc.ErrorIsNil)

	c.Check("name", s.skewed.Holder(), "holder")
	c.Check("name", s.skewed.Expiry(), s.latestValid())
}

func (s *ClientRemoteSuite) TestExtendRemoteLeaseSimpleExtend(c *gc.C) {
	leaseDuration := 10 * time.Minute
	err := s.skewed.Client.ExtendLease("name", lease.Request{"holder", leaseDuration})
	c.Check(err, jc.ErrorIsNil)

	c.Check("name", s.skewed.Holder(), "holder")
	expectExpiry := s.skewed.Clock.Now().Add(leaseDuration)
	c.Check("name", s.skewed.Expiry(), expectExpiry)
}

func (s *ClientRemoteSuite) TestExtendRemoteLeasePaddedExtend(c *gc.C) {
	needsPadding := s.lease - s.readTime
	err := s.skewed.Client.ExtendLease("name", lease.Request{"holder", needsPadding})
	c.Check(err, jc.ErrorIsNil)

	c.Check("name", s.skewed.Holder(), "holder")
	c.Check("name", s.skewed.Expiry(), s.latestValid())
}

func (s *ClientRemoteSuite) TestCannotExpireRemoteLeaseEarly(c *gc.C) {
	s.skewed.Clock.Reset(s.latestValid(), 0)
	err := s.skewed.Client.ExpireLease("name")
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientRemoteSuite) TestCanExpireRemoteLease(c *gc.C) {
	s.skewed.Clock.Reset(s.latestValid().Add(time.Nanosecond), 0)
	err := s.skewed.Client.ExpireLease("name")
	c.Check(err, jc.ErrorIsNil)
}

// ------------------------------------
