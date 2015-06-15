// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/lease"
)

// ClientValidationSuite sends bad data into all of Client's methods.
type ClientValidationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientValidationSuite{})

func (s *ClientValidationSuite) TestNewClientId(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	fix.Config.Id = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid id: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientNamespace(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	fix.Config.Namespace = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid namespace: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientCollection(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	fix.Config.Collection = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid collection: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientMongo(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	fix.Config.Mongo = nil
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing mongo")
}

func (s *ClientValidationSuite) TestNewClientClock(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	fix.Config.Clock = nil
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing clock")
}

func (s *ClientValidationSuite) TestClaimLeaseName(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("$name", lease.Request{"holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestClaimLeaseHolder(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("name", lease.Request{"$holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestClaimLeaseDuration(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("name", lease.Request{"holder", 0})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *ClientValidationSuite) TestExtendLeaseName(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ExtendLease("$name", lease.Request{"holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestExtendLeaseHolder(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ExtendLease("name", lease.Request{"$holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestExtendLeaseDuration(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ExtendLease("name", lease.Request{"holder", 0})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *ClientValidationSuite) TestExpireLeaseName(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ExpireLease("$name")
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

// ClientOperationSuite claims, extends, and expires leases.
type ClientOperationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientOperationSuite{})

func (s *ClientOperationSuite) TestClaimLease(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})

	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed, for an exact duration.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.EarliestExpiry(), exactExpiry)
	c.Check("name", fix.LatestExpiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestClaimMultipleLeases(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})

	err := fix.Client.ClaimLease("short", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Client.ClaimLease("medium", lease.Request{"grasper", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Client.ClaimLease("long", lease.Request{"clutcher", time.Hour})
	c.Assert(err, jc.ErrorIsNil)

	check := func(name, holder string, duration time.Duration) {
		c.Check(name, fix.Holder(), holder)
		expiry := fix.Zero.Add(duration)
		c.Check(name, fix.EarliestExpiry(), expiry)
		c.Check(name, fix.LatestExpiry(), expiry)
	}
	check("short", "holder", time.Second)
	check("medium", "grasper", time.Minute)
	check("long", "clutcher", time.Hour)
}

func (s *ClientOperationSuite) TestCannotClaimLeaseTwice(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})

	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is claimed and cannot be claimed again...
	err = fix.Client.ClaimLease("name", lease.Request{"other-holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even for the same holder...
	err = fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// ...not even when the lease has expired.
	fix.Clock.Advance(time.Hour)
	err = fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Check(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExtendLease(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Client.ExtendLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended, *to* (not by) the exact duration requested.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.EarliestExpiry(), exactExpiry)
	c.Check("name", fix.LatestExpiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestCanExtendStaleLease(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// Advance the clock past lease expiry time, then extend.
	fix.Clock.Advance(time.Minute)
	extendTime := fix.Clock.Now()
	leaseDuration := time.Minute
	err = fix.Client.ExtendLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// The lease is extended fine, *to* (not by) the exact duration requested.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := extendTime.Add(leaseDuration)
	c.Check("name", fix.EarliestExpiry(), exactExpiry)
	c.Check("name", fix.LatestExpiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestExtendLeaseCannotChangeHolder(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ClaimLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	leaseDuration := time.Minute
	err = fix.Client.ExtendLease("name", lease.Request{"other-holder", leaseDuration})
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExtendLeaseCannotShortenLease(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// A non-extension will succeed -- we can still honour all guarantees
	// implied by a nil error...
	err = fix.Client.ExtendLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)

	// ...but we can't make it any shorter, lest we fail to honour the
	// guarantees implied by the original lease.
	c.Check("name", fix.Holder(), "holder")
	exactExpiry := fix.Zero.Add(leaseDuration)
	c.Check("name", fix.EarliestExpiry(), exactExpiry)
	c.Check("name", fix.LatestExpiry(), exactExpiry)
}

func (s *ClientOperationSuite) TestCannotExpireLeaseBeforeExpiry(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can't be expired until after LatestExpiry.
	fix.Clock.Advance(leaseDuration)
	err = fix.Client.ExpireLease("name")
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

func (s *ClientOperationSuite) TestExpireLeaseAfterExpiry(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// It can be expired as soon as we pass LatestExpiry.
	fix.Clock.Advance(leaseDuration + time.Nanosecond)
	err = fix.Client.ExpireLease("name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check("name", fix.Vacant())
}

func (s *ClientOperationSuite) TestCannotExpireUnheldLease(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	err := fix.Client.ExpireLease("name")
	c.Assert(err, gc.Equals, lease.ErrInvalid)
}

// ------------------------------------

// ClientPersistenceSuite checks that the operations really affect the DB in
// the expected way.
type ClientPersistenceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientPersistenceSuite{})

func (s *ClientPersistenceSuite) TestNewClientInvalidClockDoc(c *gc.C) {
	config := lease.ClientConfig{
		Id:         "client",
		Namespace:  "namespace",
		Collection: "collection",
		Mongo:      NewMongo(s.db),
		Clock:      lease.SystemClock{},
	}
	dbKey := "clock#namespace#"
	err := s.db.C("collection").Insert(bson.M{"_id": dbKey})
	c.Assert(err, jc.ErrorIsNil)

	client, err := lease.NewClient(config)
	c.Check(client, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "corrupt clock document: invalid type \"\"")
}

func (s *ClientPersistenceSuite) TestNewClientInvalidLeaseDoc(c *gc.C) {
	config := lease.ClientConfig{
		Id:         "client",
		Namespace:  "namespace",
		Collection: "collection",
		Mongo:      NewMongo(s.db),
		Clock:      lease.SystemClock{},
	}
	err := s.db.C("collection").Insert(bson.M{
		"_id":       "snagglepuss",
		"type":      "lease",
		"namespace": "namespace",
	})
	c.Assert(err, jc.ErrorIsNil)

	client, err := lease.NewClient(config)
	c.Check(client, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "corrupt lease document \"snagglepuss\": inconsistent _id")
}

func (s *ClientPersistenceSuite) TestNewClientMissingClockDoc(c *gc.C) {
	fix := NewFixture(c, s.db, FixtureParams{})
	// That was the test, actually, but let's check something anyway,
	// so as not to feel too inadequate.
	c.Check("name", fix.Vacant())
}

func (s *ClientPersistenceSuite) TestClaimLease(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees exact same lease.
	fix2 := NewFixture(c, s.db, FixtureParams{})
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check("name", fix2.EarliestExpiry(), exactExpiry)
	c.Check("name", fix2.LatestExpiry(), exactExpiry)
}

func (s *ClientPersistenceSuite) TestExtendLease(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)
	leaseDuration := time.Minute
	err = fix1.Client.ExtendLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees exact same lease.
	fix2 := NewFixture(c, s.db, FixtureParams{})
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check("name", fix2.EarliestExpiry(), exactExpiry)
	c.Check("name", fix2.LatestExpiry(), exactExpiry)
}

func (s *ClientPersistenceSuite) TestExpireLease(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)
	fix1.Clock.Advance(leaseDuration + time.Nanosecond)
	err = fix1.Client.ExpireLease("name")
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees no lease.
	fix2 := NewFixture(c, s.db, FixtureParams{})
	c.Check("name", fix2.Vacant())
}

func (s *ClientPersistenceSuite) TestNamespaceIsolation(c *gc.C) {
	fix1 := NewFixture(c, s.db, FixtureParams{})
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, different namespace: sees no lease.
	fix2 := NewFixture(c, s.db, FixtureParams{
		Namespace: "different-namespace",
	})
	c.Check("name", fix2.Vacant())
}

// ------------------------------------

// ClientRemoteSuite checks that clients do not break one another's promises.
type ClientRemoteSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientRemoteSuite{})

func (s *ClientRemoteSuite) TestReadSkew(c *gc.C) {
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

func (s *ClientRemoteSuite) TestExtendRemoteLeaseNoop(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientRemoteSuite) TestExtendRemoteLeaseSimpleExtend(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientRemoteSuite) TestExtendRemoteLeasePaddedExtend(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientRemoteSuite) TestExpireRemoteLease(c *gc.C) {
	c.Fatalf("not done")
}

// ------------------------------------

// ClientRaceSuite tests the ugliest of details.
type ClientRaceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientRaceSuite{})

func (s *ClientRaceSuite) TestMany(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientRaceSuite) TestManyMany(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientRaceSuite) TestMore(c *gc.C) {
	c.Fatalf("not done")
}
