// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/lease"
)

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
		Clock:      clock.WallClock,
	}
	dbKey := "clock#namespace#"
	err := s.db.C("collection").Insert(bson.M{"_id": dbKey})
	c.Assert(err, jc.ErrorIsNil)

	client, err := lease.NewClient(config)
	c.Check(client, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `corrupt clock document: invalid type ""`)
}

func (s *ClientPersistenceSuite) TestNewClientInvalidLeaseDoc(c *gc.C) {
	config := lease.ClientConfig{
		Id:         "client",
		Namespace:  "namespace",
		Collection: "collection",
		Mongo:      NewMongo(s.db),
		Clock:      clock.WallClock,
	}
	err := s.db.C("collection").Insert(bson.M{
		"_id":       "snagglepuss",
		"type":      "lease",
		"namespace": "namespace",
	})
	c.Assert(err, jc.ErrorIsNil)

	client, err := lease.NewClient(config)
	c.Check(client, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `corrupt lease document "snagglepuss": inconsistent _id`)
}

func (s *ClientPersistenceSuite) TestNewClientMissingClockDoc(c *gc.C) {
	// The database starts out empty, so just creating the fixture is enough
	// to test this code path.
	s.EasyFixture(c)
}

func (s *ClientPersistenceSuite) TestNewClientExtantClockDoc(c *gc.C) {
	// Empty database: new Client creates clock doc.
	s.EasyFixture(c)

	// Clock doc exists; new Client created successfully.
	s.EasyFixture(c)
}

func (s *ClientPersistenceSuite) TestClaimLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees exact same lease.
	fix2 := s.EasyFixture(c)
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check("name", fix2.Expiry(), exactExpiry)
}

func (s *ClientPersistenceSuite) TestExtendLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", time.Second})
	c.Assert(err, jc.ErrorIsNil)
	leaseDuration := time.Minute
	err = fix1.Client.ExtendLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees exact same lease.
	fix2 := s.EasyFixture(c)
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check("name", fix2.Expiry(), exactExpiry)
}

func (s *ClientPersistenceSuite) TestExpireLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)
	fix1.Clock.Advance(leaseDuration + time.Nanosecond)
	err = fix1.Client.ExpireLease("name")
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, new instance: sees no lease.
	fix2 := s.EasyFixture(c)
	c.Check("name", fix2.Holder(), "")
}

func (s *ClientPersistenceSuite) TestNamespaceIsolation(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client id, same clock, different namespace: sees no lease.
	fix2 := s.NewFixture(c, FixtureParams{
		Namespace: "different-namespace",
	})
	c.Check("name", fix2.Holder(), "")
}

func (s *ClientPersistenceSuite) TestTimezoneChanges(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Same client can come up in a different timezone and still work correctly.
	fix2 := s.NewFixture(c, FixtureParams{
		ClockStart: fix1.Zero.In(time.FixedZone("somewhere", -1234)),
	})
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix2.Zero.Add(leaseDuration)
	c.Check("name", fix2.Expiry(), exactExpiry)
}

func (s *ClientPersistenceSuite) TestTimezoneIsolation(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Client.ClaimLease("name", lease.Request{"holder", leaseDuration})
	c.Assert(err, jc.ErrorIsNil)

	// Different client *and* different timezone; but clock agrees perfectly,
	// so we still see no skew.
	fix2 := s.NewFixture(c, FixtureParams{
		Id:         "remote-client",
		ClockStart: fix1.Zero.UTC(),
	})
	c.Check("name", fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration).UTC()
	c.Check("name", fix2.Expiry(), exactExpiry)
}
