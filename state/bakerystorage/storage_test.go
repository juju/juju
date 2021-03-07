// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"context"
	"encoding/json"
	"time" // Only used for time types.

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/mgorootkeystore"
	"github.com/juju/mgo/v2"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
)

type StorageSuite struct {
	testing.BaseSuite
	gitjujutesting.Stub
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
		GetStorage: func(rootKeys *mgorootkeystore.RootKeys, coll mongo.Collection, expireAfter time.Duration) bakery.RootKeyStore {
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
	s.CheckCalls(c, []gitjujutesting.StubCall{
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
	s.CheckCalls(c, []gitjujutesting.StubCall{
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
	*gitjujutesting.Stub

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
	*gitjujutesting.Stub
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
