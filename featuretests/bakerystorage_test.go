// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/bakerystorage"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"
)

// This suite is not about a feature tests per se, but tests the integration
// of the mongo-based bakery storage with the macaroon bakery service.
type BakeryStorageSuite struct {
	gitjujutesting.MgoSuite

	store   bakery.Storage
	service *bakery.Service
	db      *mgo.Database
	coll    *mgo.Collection
}

func (s *BakeryStorageSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)

	s.db = s.Session.DB("bakerydb")
	s.coll = s.db.C("bakedgoods")

	s.initService(c, time.Minute)
}

func (s *BakeryStorageSuite) initService(c *gc.C, expiryTime time.Duration) {
	store, err := bakerystorage.New(bakerystorage.Config{
		GetCollection: func(name string) (mongo.Collection, func()) {
			return mongo.CollectionFromName(s.db, name)
		},
		Collection:  "bakedgoods",
		Clock:       clock.WallClock,
		ExpireAfter: expiryTime,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store = store

	service, err := bakery.NewService(bakery.NewServiceParams{
		Location: "straya",
		Store:    s.store,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.service = service
}

func (s *BakeryStorageSuite) ensureIndex(c *gc.C) {
	err := s.coll.EnsureIndex(mgo.Index{
		Key:         []string{"expire-at"},
		ExpireAfter: time.Second,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BakeryStorageSuite) TestCheckNewMacaroon(c *gc.C) {
	mac, err := s.service.NewMacaroon("", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.service.CheckAny([]macaroon.Slice{{mac}}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BakeryStorageSuite) TestExpiryTime(c *gc.C) {
	// Reinitialise bakery service with storage that will expire
	// items after 1 second.
	s.initService(c, time.Second)
	s.ensureIndex(c)

	mac, err := s.service.NewMacaroon("", nil, nil)
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
