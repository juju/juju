// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"context"
	"time"

	"github.com/juju/mgo/v2"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v3/bakery"
	"gopkg.in/macaroon-bakery.v3/bakery/checkers"
	"gopkg.in/macaroon-bakery.v3/bakery/mgorootkeystore"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
)

// This suite is not about a feature tests per se, but tests the integration
// of the mongo-based bakery storage with the macaroon bakery service.
type BakeryStorageSuite struct {
	gitjujutesting.MgoSuite
	gitjujutesting.LoggingSuite

	store  bakerystorage.ExpirableStorage
	bakery *bakery.Bakery
	db     *mgo.Database
	coll   *mgo.Collection
}

func (s *BakeryStorageSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.db = s.Session.DB("bakerydb")
	s.coll = s.db.C("bakedgoods")
	s.ensureIndexes(c)
	s.initService(c, false)
}

func (s *BakeryStorageSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *BakeryStorageSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BakeryStorageSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *BakeryStorageSuite) initService(c *gc.C, enableExpiry bool) {
	store, err := bakerystorage.New(bakerystorage.Config{
		GetCollection: func() (mongo.Collection, func()) {
			return mongo.CollectionFromName(s.db, s.coll.Name)
		},
		GetStorage: func(rootKeys *mgorootkeystore.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.RootKeyStore {
			return rootKeys.NewStore(coll.Writeable().Underlying(), mgorootkeystore.Policy{
				ExpiryDuration: expireAfter,
			})
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	if enableExpiry {
		store = store.ExpireAfter(10 * time.Second)
	}
	s.store = store
	s.bakery = bakery.New(bakery.BakeryParams{
		RootKeyStore: s.store,
	})
}

func (s *BakeryStorageSuite) ensureIndexes(c *gc.C) {
	for _, index := range bakerystorage.MongoIndexes() {
		err := s.coll.EnsureIndex(index)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *BakeryStorageSuite) TestCheckNewMacaroon(c *gc.C) {
	cav := []checkers.Caveat{{Condition: "something"}}
	mac, err := s.bakery.Oven.NewMacaroon(context.TODO(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.bakery.Oven.VerifyMacaroon(context.TODO(), macaroon.Slice{mac.M()})
	c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")

	store := s.store.ExpireAfter(10 * time.Second)
	b := bakery.New(bakery.BakeryParams{
		RootKeyStore: store,
	})
	mac, err = b.Oven.NewMacaroon(context.TODO(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, jc.ErrorIsNil)
	op, conditions, err := s.bakery.Oven.VerifyMacaroon(context.TODO(), macaroon.Slice{mac.M()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, jc.DeepEquals, []bakery.Op{bakery.NoOp})
	c.Assert(conditions, jc.DeepEquals, []string{"something"})
}

func (s *BakeryStorageSuite) TestExpiryTime(c *gc.C) {
	// Reinitialise bakery service with storage that will expire
	// items immediately.
	s.initService(c, true)

	mac, err := s.bakery.Oven.NewMacaroon(context.TODO(), bakery.LatestVersion, nil, bakery.NoOp)
	c.Assert(err, jc.ErrorIsNil)

	// The background thread that removes records runs every 60s.
	// Give a little bit of leeway for loaded systems.
	for i := 0; i < 90; i++ {
		_, _, err = s.bakery.Oven.VerifyMacaroon(context.TODO(), macaroon.Slice{mac.M()})
		if err == nil {
			time.Sleep(time.Second)
			continue
		}
		c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")
		return
	}
	c.Fatal("timed out waiting for storage expiry")
}
