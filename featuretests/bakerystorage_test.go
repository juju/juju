// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/macaroon.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
)

// This suite is not about a feature tests per se, but tests the integration
// of the mongo-based bakery storage with the macaroon bakery service.
type BakeryStorageSuite struct {
	gitjujutesting.MgoSuite
	gitjujutesting.LoggingSuite

	store   bakerystorage.ExpirableStorage
	service *bakery.Service
	db      *mgo.Database
	coll    *mgo.Collection
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
		GetStorage: func(rootKeys *mgostorage.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.Storage {
			return rootKeys.NewStorage(coll.Writeable().Underlying(), mgostorage.Policy{
				ExpiryDuration: expireAfter,
			})
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	if enableExpiry {
		store = store.ExpireAfter(10 * time.Second)
	}
	s.store = store

	service, err := bakery.NewService(bakery.NewServiceParams{
		Location: "straya",
		Store:    s.store,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.service = service
}

func (s *BakeryStorageSuite) ensureIndexes(c *gc.C) {
	for _, index := range bakerystorage.MongoIndexes() {
		err := s.coll.EnsureIndex(index)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *BakeryStorageSuite) TestCheckNewMacaroon(c *gc.C) {
	mac, err := s.service.NewMacaroon(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.service.CheckAny([]macaroon.Slice{{mac}}, nil, nil)
	c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")

	store := s.store.ExpireAfter(10 * time.Second)
	mac, err = s.service.WithStore(store).NewMacaroon(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.service.CheckAny([]macaroon.Slice{{mac}}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BakeryStorageSuite) TestExpiryTime(c *gc.C) {
	// Reinitialise bakery service with storage that will expire
	// items immediately.
	s.initService(c, true)

	mac, err := s.service.NewMacaroon(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The background thread that removes records runs every 60s.
	// Give a little bit of leeway for loaded systems.
	for i := 0; i < 90; i++ {
		_, err = s.service.CheckAny([]macaroon.Slice{{mac}}, nil, nil)
		if err == nil {
			time.Sleep(time.Second)
			continue
		}
		c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")
		return
	}
	c.Fatal("timed out waiting for storage expiry")
}
