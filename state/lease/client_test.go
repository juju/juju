// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/lease"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
	jujutesting.MgoSuite
	db *mgo.Database
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *ClientSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.db = s.Session.DB("juju")
}

func (s *ClientSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *ClientSuite) TestNewClientBadConfig(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientSuite) TestNewClientInvalidClockDoc(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientSuite) TestNewClientMissingClockDoc(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientSuite) TestNewClientValidClockDoc(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientSuite) TestClaimLeaseCached(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The instance that claimed the lease has the lease cached.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.EarliestExpiry(), exactExpiry)
	c.Check("name", fix.LatestExpiry(), exactExpiry)
}

func (s *ClientSuite) TestClaimLeasePersisted(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, same everything, but new instance; no change.
	fix2 := NewFixture(c, s.db, FixtureParams{})
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check("name", fix2.EarliestExpiry(), exactExpiry)
	c.Check("name", fix2.LatestExpiry(), exactExpiry)
}

func (s *ClientSuite) TestClaimLeaseRemoteSkew(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Remote client, possibly reading in the future and possibly just ahead
	// by a second, taking 100ms to read the clock doc; sees same lease with
	// suitable uncertainty.
	offset := time.Second
	readDuration := 100 * time.Millisecond
	fix2 := NewFixture(c, s.db, FixtureParams{
		Id:         "remote-client",
		ClockStart: fix1.Zero.Add(offset),
		ClockStep:  readDuration,
	})
	c.Check("name", fix2.Holder(), "holder")
	earliestExpiry := fix1.Zero.Add(offset + leaseDuration)
	c.Check("name", fix2.EarliestExpiry(), earliestExpiry)
	c.Check("name", fix2.LatestExpiry(), earliestExpiry.Add(readDuration))
}

func (s *ClientSuite) TestNotDone(c *gc.C) {
	c.Fatalf("not done")
}
