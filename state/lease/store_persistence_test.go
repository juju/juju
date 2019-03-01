// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/state/lease"
)

// StorePersistenceSuite checks that the operations really affect the DB in
// the expected way.
type StorePersistenceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&StorePersistenceSuite{})

func (s *StorePersistenceSuite) TestNewStoreInvalidLeaseDoc(c *gc.C) {
	config := lease.StoreConfig{
		Id:          "store",
		Namespace:   "namespace",
		ModelUUID:   "model-uuid",
		Collection:  "collection",
		Mongo:       NewMongo(s.db),
		LocalClock:  clock.WallClock,
		GlobalClock: GlobalClock{},
	}
	err := s.db.C("collection").Insert(bson.M{
		"_id":       "snagglepuss",
		"namespace": "namespace",
	})
	c.Assert(err, jc.ErrorIsNil)

	store, err := lease.NewStore(config)
	c.Check(store, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `corrupt lease document "snagglepuss": inconsistent _id`)
}

func (s *StorePersistenceSuite) TestNewStoreMissingClockDoc(c *gc.C) {
	// The database starts out empty, so just creating the fixture is enough
	// to test this code path.
	s.EasyFixture(c)
}

func (s *StorePersistenceSuite) TestNewStoreExtantClockDoc(c *gc.C) {
	// Empty database: new Store creates clock doc.
	s.EasyFixture(c)

	// Clock doc exists; new Store created successfully.
	s.EasyFixture(c)
}

func (s *StorePersistenceSuite) TestClaimLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Same store id, same clock, new instance: sees exact same lease.
	fix2 := s.EasyFixture(c)
	c.Check(key("name"), fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check(key("name"), fix2.Expiry(), exactExpiry)
}

func (s *StorePersistenceSuite) TestExtendLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", time.Second}, nil)
	c.Assert(err, jc.ErrorIsNil)
	leaseDuration := time.Minute
	err = fix1.Store.ExtendLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Same store id, same clock, new instance: sees exact same lease.
	fix2 := s.EasyFixture(c)
	c.Check(key("name"), fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration)
	c.Check(key("name"), fix2.Expiry(), exactExpiry)
}

func (s *StorePersistenceSuite) TestExpireLease(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)
	fix1.GlobalClock.Advance(leaseDuration + time.Nanosecond)
	err = fix1.Store.ExpireLease(key("name"))
	c.Assert(err, jc.ErrorIsNil)

	// Same store id, same clock, new instance: sees no lease.
	fix2 := s.EasyFixture(c)
	c.Check(key("name"), fix2.Holder(), "")
}

func (s *StorePersistenceSuite) TestNamespaceIsolation(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Same store id, same clock, different namespace: sees no lease.
	fix2 := s.NewFixture(c, FixtureParams{
		Namespace: "different-namespace",
	})
	c.Check(key("name"), fix2.Holder(), "")
}

func (s *StorePersistenceSuite) TestTimezoneChanges(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Same store can come up in a different timezone and still work correctly.
	fix2 := s.NewFixture(c, FixtureParams{
		LocalClockStart: fix1.Zero.In(time.FixedZone("somewhere", -1234)),
	})
	c.Check(key("name"), fix2.Holder(), "holder")
	exactExpiry := fix2.Zero.Add(leaseDuration)
	c.Check(key("name"), fix2.Expiry(), exactExpiry)
}

func (s *StorePersistenceSuite) TestTimezoneIsolation(c *gc.C) {
	fix1 := s.EasyFixture(c)
	leaseDuration := time.Minute
	err := fix1.Store.ClaimLease(key("name"), corelease.Request{"holder", leaseDuration}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Different store *and* different timezone; but clock agrees perfectly,
	// so we still see no skew.
	fix2 := s.NewFixture(c, FixtureParams{
		Id:              "remote-store",
		LocalClockStart: fix1.Zero.UTC(),
	})
	c.Check(key("name"), fix2.Holder(), "holder")
	exactExpiry := fix1.Zero.Add(leaseDuration).UTC()
	c.Check(key("name"), fix2.Expiry(), exactExpiry)
}
