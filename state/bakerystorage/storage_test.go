// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"context"
	"encoding/json"
	"time" // Only used for time types.

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/mgo/v3"
	mgotesting "github.com/juju/mgo/v3/testing"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/testing"
)

type StorageSuite struct {
	testing.BaseSuite
	jujutesting.Stub
	collection      mockCollection
	memStorage      bakery.RootKeyStore
	closeCollection func()
	config          Config
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.collection = mockCollection{
		Stub: &s.Stub,
		one: func(q *mockQuery, result *interface{}) error {
			location := q.id.(string)
			if location != "oldkey" {
				return mgo.ErrNotFound
			}
			*(*result).(*storageDoc) = storageDoc{
				Location: q.id.(string),
				Item:     "{\"RootKey\":\"ibbhlQv5+yf7UMNI77W4hxQeQjRdMxs0\"}",
			}
			return nil
		},
	}
	s.closeCollection = func() {
		s.AddCall("Close")
		s.PopNoErr()
	}
	s.memStorage = bakery.NewMemRootKeyStore()
	s.config = Config{
		GetCollection: func() (mongo.Collection, func()) {
			s.AddCall("GetCollection")
			s.PopNoErr()
			return &s.collection, s.closeCollection
		},
		GetStorage: func(rootKeys *RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.RootKeyStore {
			s.AddCall("GetStorage", coll, expireAfter)
			s.PopNoErr()
			return s.memStorage
		},
	}
}

func (s *StorageSuite) TestValidateConfigGetCollection(c *gc.C) {
	s.config.GetCollection = nil
	_, err := New(s.config)
	c.Assert(err, gc.ErrorMatches, "validating config: nil GetCollection not valid")
}

func (s *StorageSuite) TestValidateConfigGetStorage(c *gc.C) {
	s.config.GetStorage = nil
	_, err := New(s.config)
	c.Assert(err, gc.ErrorMatches, "validating config: nil GetStorage not valid")
}

func (s *StorageSuite) TestExpireAfter(c *gc.C) {
	store, err := New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	store = store.ExpireAfter(24 * time.Hour)
	c.Assert(ExpireAfter(store), gc.Equals, 24*time.Hour)
}

func (s *StorageSuite) TestGet(c *gc.C) {
	store, err := New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	rootKey, id, err := store.RootKey(ctx)
	c.Assert(err, jc.ErrorIsNil)

	item, err := store.Get(ctx, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(item, jc.DeepEquals, rootKey)
	s.CheckCalls(c, []jujutesting.StubCall{
		{"GetCollection", nil},
		{"GetStorage", []interface{}{&s.collection, time.Duration(0)}},
		{"Close", nil},
		{"GetCollection", nil},
		{"GetStorage", []interface{}{&s.collection, time.Duration(0)}},
		{"Close", nil},
	})
}

func (s *StorageSuite) TestGetNotFound(c *gc.C) {
	store, err := New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	c.Log("1.")
	_, err = store.Get(context.Background(), []byte("foo"))
	c.Log("2.")
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
}

func (s *StorageSuite) TestGetLegacyFallback(c *gc.C) {
	store, err := New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	var rk legacyRootKey
	err = json.Unmarshal([]byte("{\"RootKey\":\"ibbhlQv5+yf7UMNI77W4hxQeQjRdMxs0\"}"), &rk)
	c.Assert(err, jc.ErrorIsNil)

	item, err := store.Get(context.Background(), []byte("oldkey"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(item, jc.DeepEquals, rk.RootKey)
	s.CheckCalls(c, []jujutesting.StubCall{
		{"GetCollection", nil},
		{"GetStorage", []interface{}{&s.collection, time.Duration(0)}},
		{"GetCollection", nil},
		{"FindId", []interface{}{"oldkey"}},
		{"One", []interface{}{&storageDoc{
			// Set by mock, not in input. Unimportant anyway.
			Location: "oldkey",
			Item:     "{\"RootKey\":\"ibbhlQv5+yf7UMNI77W4hxQeQjRdMxs0\"}",
		}}},
		{"Close", nil},
		{"Close", nil},
	})
}

type mockCollection struct {
	mongo.WriteCollection
	*jujutesting.Stub

	one func(q *mockQuery, result *interface{}) error
}

func (c *mockCollection) FindId(id interface{}) mongo.Query {
	c.MethodCall(c, "FindId", id)
	c.PopNoErr()
	return &mockQuery{Stub: c.Stub, id: id, one: c.one}
}

func (c *mockCollection) Writeable() mongo.WriteCollection {
	c.MethodCall(c, "Writeable")
	c.PopNoErr()
	return c
}

type mockQuery struct {
	mongo.Query
	*jujutesting.Stub
	id  interface{}
	one func(q *mockQuery, result *interface{}) error
}

func (q *mockQuery) One(result interface{}) error {
	q.MethodCall(q, "One", result)

	err := q.one(q, &result)
	if err != nil {
		return err
	}
	return q.NextErr()
}

var _ = gc.Suite(&BakeryStorageSuite{})

type BakeryStorageSuite struct {
	mgotesting.MgoSuite
	jujutesting.LoggingSuite

	store  ExpirableStorage
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
	store, err := New(Config{
		GetCollection: func() (mongo.Collection, func()) {
			return mongo.CollectionFromName(s.db, s.coll.Name)
		},
		GetStorage: func(rootKeys *RootKeys, coll mongo.Collection, expireAfter time.Duration) (storage bakery.RootKeyStore) {
			return rootKeys.NewStore(coll.Writeable().Underlying(), Policy{
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
	for _, index := range MongoIndexes() {
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
