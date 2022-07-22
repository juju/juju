// Copyright 2014-2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"context"
	"fmt"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/mgo/v3"
	mgotesting "github.com/juju/mgo/v3/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type RootKeySuite struct {
	testing.BaseSuite
	mgotesting.MgoSuite
}

var _ = gc.Suite(&RootKeySuite{})

func (s *RootKeySuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *RootKeySuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *RootKeySuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)
}

func (s *RootKeySuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

var epoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

var isValidWithPolicyTests = []struct {
	about  string
	policy Policy
	now    time.Time
	key    dbrootkeystore.RootKey
	expect bool
}{{
	about: "success",
	policy: Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   3 * time.Minute,
	},
	now: epoch.Add(20 * time.Minute),
	key: dbrootkeystore.RootKey{
		Created: epoch.Add(19 * time.Minute),
		Expires: epoch.Add(24 * time.Minute),
		Id:      []byte("id"),
		RootKey: []byte("key"),
	},
	expect: true,
}, {
	about: "empty root key",
	policy: Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   3 * time.Minute,
	},
	now:    epoch.Add(20 * time.Minute),
	key:    dbrootkeystore.RootKey{},
	expect: false,
}, {
	about: "created too early",
	policy: Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   3 * time.Minute,
	},
	now: epoch.Add(20 * time.Minute),
	key: dbrootkeystore.RootKey{
		Created: epoch.Add(18*time.Minute - time.Millisecond),
		Expires: epoch.Add(24 * time.Minute),
		Id:      []byte("id"),
		RootKey: []byte("key"),
	},
	expect: false,
}, {
	about: "expires too early",
	policy: Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   3 * time.Minute,
	},
	now: epoch.Add(20 * time.Minute),
	key: dbrootkeystore.RootKey{
		Created: epoch.Add(19 * time.Minute),
		Expires: epoch.Add(21 * time.Minute),
		Id:      []byte("id"),
		RootKey: []byte("key"),
	},
	expect: false,
}, {
	about: "expires too late",
	policy: Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   3 * time.Minute,
	},
	now: epoch.Add(20 * time.Minute),
	key: dbrootkeystore.RootKey{
		Created: epoch.Add(19 * time.Minute),
		Expires: epoch.Add(25*time.Minute + time.Millisecond),
		Id:      []byte("id"),
		RootKey: []byte("key"),
	},
	expect: false,
}}

func (s *RootKeySuite) TestIsValidWithPolicy(c *gc.C) {
	var now time.Time
	s.PatchValue(&clock, clockVal(&now))
	for i, test := range isValidWithPolicyTests {
		c.Logf("test %d: %v", i, test.about)
		c.Assert(test.key.IsValidWithPolicy(dbrootkeystore.Policy(test.policy), test.now), gc.Equals, test.expect)
	}
}

func (s *RootKeySuite) TestRootKeyUsesKeysValidWithPolicy(c *gc.C) {
	// We re-use the TestIsValidWithPolicy tests so that we
	// know that the mongo logic uses the same behaviour.
	var now time.Time
	s.PatchValue(&clock, clockVal(&now))
	for _, test := range isValidWithPolicyTests {
		if test.key.RootKey == nil {
			// We don't store empty root keys in the database.
			c.Log("skipping test with empty root key")
			continue
		}
		coll := s.testColl(c)
		// Prime the collection with the root key document.
		err := coll.Insert(test.key)
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))

		store := NewRootKeys(10).NewStore(coll, test.policy)
		now = test.now
		key, id, err := store.RootKey(context.Background())
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
		if test.expect {
			c.Assert(string(id), gc.Equals, "id", gc.Commentf(test.about))
			c.Assert(string(key), gc.Equals, "key", gc.Commentf(test.about))
		} else {
			// If it didn't match then RootKey will have
			// generated a new key.
			c.Assert(key, gc.HasLen, 24, gc.Commentf(test.about))
			c.Assert(id, gc.HasLen, 32, gc.Commentf(test.about))
		}
		err = coll.DropCollection()
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
	}
}

func (s *RootKeySuite) TestRootKey(c *gc.C) {
	now := epoch
	s.PatchValue(&clock, clockVal(&now))
	coll := s.testColl(c)

	store := NewRootKeys(10).NewStore(coll, Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   5 * time.Minute,
	})
	key, id, err := store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key, gc.HasLen, 24)
	c.Assert(id, gc.HasLen, 32)

	// If we get a key within the generate interval, we should
	// get the same one.
	now = epoch.Add(time.Minute)
	key1, id1, err := store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key1, gc.DeepEquals, key)
	c.Assert(id1, gc.DeepEquals, id)

	// A different store instance should get the same root key.
	store1 := NewRootKeys(10).NewStore(coll, Policy{
		GenerateInterval: 2 * time.Minute,
		ExpiryDuration:   5 * time.Minute,
	})
	key1, id1, err = store1.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key1, gc.DeepEquals, key)
	c.Assert(id1, gc.DeepEquals, id)

	// After the generation interval has passed, we should generate a new key.
	now = epoch.Add(2*time.Minute + time.Second)
	key1, id1, err = store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key, gc.HasLen, 24)
	c.Assert(id, gc.HasLen, 32)
	c.Assert(key1, gc.Not(gc.DeepEquals), key)
	c.Assert(id1, gc.Not(gc.DeepEquals), id)

	// The other store should pick it up too.
	key2, id2, err := store1.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key2, gc.DeepEquals, key1)
	c.Assert(id2, gc.DeepEquals, id1)
}

func (s *RootKeySuite) TestRootKeyDefaultGenerateInterval(c *gc.C) {
	now := epoch
	s.PatchValue(&clock, clockVal(&now))
	coll := s.testColl(c)
	store := NewRootKeys(10).NewStore(coll, Policy{
		ExpiryDuration: 5 * time.Minute,
	})
	key, id, err := store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)

	now = epoch.Add(5 * time.Minute)
	key1, id1, err := store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(key1, gc.DeepEquals, key)
	c.Assert(id1, gc.DeepEquals, id)

	now = epoch.Add(5*time.Minute + time.Millisecond)
	key1, id1, err = store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(string(key1), gc.Not(gc.Equals), string(key))
	c.Assert(string(id1), gc.Not(gc.Equals), string(id))
}

var preferredRootKeyTests = []struct {
	about    string
	now      time.Time
	keys     []dbrootkeystore.RootKey
	policy   Policy
	expectId []byte
}{{
	about: "latest creation time is preferred",
	now:   epoch.Add(5 * time.Minute),
	keys: []dbrootkeystore.RootKey{{
		Created: epoch.Add(4 * time.Minute),
		Expires: epoch.Add(15 * time.Minute),
		Id:      []byte("id0"),
		RootKey: []byte("key0"),
	}, {
		Created: epoch.Add(5*time.Minute + 30*time.Second),
		Expires: epoch.Add(16 * time.Minute),
		Id:      []byte("id1"),
		RootKey: []byte("key1"),
	}, {
		Created: epoch.Add(5 * time.Minute),
		Expires: epoch.Add(16 * time.Minute),
		Id:      []byte("id2"),
		RootKey: []byte("key2"),
	}},
	policy: Policy{
		GenerateInterval: 5 * time.Minute,
		ExpiryDuration:   7 * time.Minute,
	},
	expectId: []byte("id1"),
}, {
	about: "ineligible keys are exluded",
	now:   epoch.Add(5 * time.Minute),
	keys: []dbrootkeystore.RootKey{{
		Created: epoch.Add(4 * time.Minute),
		Expires: epoch.Add(15 * time.Minute),
		Id:      []byte("id0"),
		RootKey: []byte("key0"),
	}, {
		Created: epoch.Add(5 * time.Minute),
		Expires: epoch.Add(16*time.Minute + 30*time.Second),
		Id:      []byte("id1"),
		RootKey: []byte("key1"),
	}, {
		Created: epoch.Add(6 * time.Minute),
		Expires: epoch.Add(time.Hour),
		Id:      []byte("id2"),
		RootKey: []byte("key2"),
	}},
	policy: Policy{
		GenerateInterval: 5 * time.Minute,
		ExpiryDuration:   7 * time.Minute,
	},
	expectId: []byte("id1"),
}}

func (s *RootKeySuite) TestPreferredRootKeyFromDatabase(c *gc.C) {
	var now time.Time
	s.PatchValue(&clock, clockVal(&now))
	for _, test := range preferredRootKeyTests {
		coll := s.testColl(c)
		for _, key := range test.keys {
			err := coll.Insert(key)
			c.Assert(err, gc.IsNil, gc.Commentf(test.about))
		}
		store := NewRootKeys(10).NewStore(coll, test.policy)
		now = test.now
		_, id, err := store.RootKey(context.Background())
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
		c.Assert(id, gc.DeepEquals, test.expectId, gc.Commentf(test.about))
		err = coll.DropCollection()
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
	}
}

func (s *RootKeySuite) TestPreferredRootKeyFromCache(c *gc.C) {
	var now time.Time
	s.PatchValue(&clock, clockVal(&now))
	for _, test := range preferredRootKeyTests {
		coll := s.testColl(c)
		for _, key := range test.keys {
			err := coll.Insert(key)
			c.Assert(err, gc.IsNil)
		}
		store := NewRootKeys(10).NewStore(coll, test.policy)
		// Ensure that all the keys are in cache by getting all of them.
		for _, key := range test.keys {
			got, err := store.Get(context.Background(), key.Id)
			c.Assert(err, gc.IsNil, gc.Commentf(test.about))
			c.Assert(got, gc.DeepEquals, key.RootKey, gc.Commentf(test.about))
		}
		// Remove all the keys from the collection so that
		// we know we must be acquiring them from the cache.
		_, err := coll.RemoveAll(nil)
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))

		// Test that RootKey returns the expected key.
		now = test.now
		_, id, err := store.RootKey(context.Background())
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
		c.Assert(id, gc.DeepEquals, test.expectId, gc.Commentf(test.about))
		err = coll.DropCollection()
		c.Assert(err, gc.IsNil, gc.Commentf(test.about))
	}
}

func (s *RootKeySuite) TestGet(c *gc.C) {
	now := epoch
	s.PatchValue(&clock, clockVal(&now))

	coll := s.testColl(c)
	store := NewRootKeys(5).NewStore(coll, Policy{
		GenerateInterval: 1 * time.Minute,
		ExpiryDuration:   30 * time.Minute,
	})
	type idKey struct {
		id  string
		key []byte
	}
	var keys []idKey
	keyIds := make(map[string]bool)
	for i := 0; i < 20; i++ {
		key, id, err := store.RootKey(context.Background())
		c.Assert(err, gc.IsNil)
		c.Assert(keyIds[string(id)], gc.Equals, false)
		keys = append(keys, idKey{string(id), key})
		now = now.Add(time.Minute + time.Second)
	}
	for i, k := range keys {
		key, err := store.Get(context.Background(), []byte(k.id))
		c.Assert(err, gc.IsNil, gc.Commentf("key %d (%s)", i, k.id))
		c.Assert(key, gc.DeepEquals, k.key, gc.Commentf("key %d (%s)", i, k.id))
	}
	// Check that the keys are cached.
	//
	// Since the cache size is 5, the most recent 5 items will be in
	// the primary cache; the 5 items before that will be in the old
	// cache and nothing else will be cached.
	//
	// The first time we fetch an item from the old cache, a new
	// primary cache will be allocated, all existing items in the
	// old cache except that item will be evicted, and all items in
	// the current primary cache moved to the old cache.
	//
	// The upshot of that is that all but the first 6 calls to Get
	// should result in a database fetch.

	var fetched []string
	s.PatchValue(&mgoCollectionFindId, func(coll *mgo.Collection, id interface{}) *mgo.Query {
		fetched = append(fetched, string(id.([]byte)))
		return coll.FindId(id)
	})
	c.Logf("testing cache")

	for i := len(keys) - 1; i >= 0; i-- {
		k := keys[i]
		key, err := store.Get(context.Background(), []byte(k.id))
		c.Assert(err, gc.IsNil)
		c.Assert(err, gc.IsNil, gc.Commentf("key %d (%s)", i, k.id))
		c.Assert(key, gc.DeepEquals, k.key, gc.Commentf("key %d (%s)", i, k.id))
	}
	c.Assert(len(fetched), gc.Equals, len(keys)-6)
	for i, id := range fetched {
		c.Assert(id, gc.Equals, keys[len(keys)-6-i-1].id)
	}
}

func (s *RootKeySuite) TestGetCachesMisses(c *gc.C) {
	coll := s.testColl(c)
	store := NewRootKeys(5).NewStore(coll, Policy{
		GenerateInterval: 1 * time.Minute,
		ExpiryDuration:   30 * time.Minute,
	})
	var fetched []string
	s.PatchValue(&mgoCollectionFindId, func(coll *mgo.Collection, id interface{}) *mgo.Query {
		fetched = append(fetched, fmt.Sprintf("%#v", id))
		return coll.FindId(id)
	})
	key, err := store.Get(context.Background(), []byte("foo"))
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
	c.Assert(key, gc.IsNil)
	// This should check twice first using a []byte second using a string
	c.Assert(fetched, gc.DeepEquals, []string{fmt.Sprintf("%#v", []byte("foo")), fmt.Sprintf("%#v", "foo")})
	fetched = nil

	key, err = store.Get(context.Background(), []byte("foo"))
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
	c.Assert(key, gc.IsNil)
	c.Assert(fetched, gc.IsNil)
}

func (s *RootKeySuite) TestGetExpiredItemFromCache(c *gc.C) {
	now := epoch
	s.PatchValue(&clock, clockVal(&now))
	coll := s.testColl(c)
	store := NewRootKeys(10).NewStore(coll, Policy{
		ExpiryDuration: 5 * time.Minute,
	})
	_, id, err := store.RootKey(context.Background())
	c.Assert(err, gc.IsNil)

	s.PatchValue(&mgoCollectionFindId, func(*mgo.Collection, interface{}) *mgo.Query {
		c.Errorf("FindId unexpectedly called")
		return nil
	})

	now = epoch.Add(15 * time.Minute)

	_, err = store.Get(context.Background(), id)
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
}

func (s *RootKeySuite) TestEnsureIndex(c *gc.C) {
	keys := NewRootKeys(5)
	coll := s.testColl(c)
	err := keys.EnsureIndex(coll)
	c.Assert(err, gc.IsNil)

	// This code can take up to 60s to run; there's no way
	// to force it to run more quickly, but it provides reassurance
	// that the code actually works.
	// Reenable the rest of this test if concerned about index behaviour.

	c.Skip("test runs too slowly")

	_, id1, err := keys.NewStore(coll, Policy{
		ExpiryDuration: 100 * time.Millisecond,
	}).RootKey(context.Background())

	c.Assert(err, gc.IsNil)

	_, id2, err := keys.NewStore(coll, Policy{
		ExpiryDuration: time.Hour,
	}).RootKey(context.Background())

	c.Assert(err, gc.IsNil)
	c.Assert(id2, gc.Not(gc.Equals), id1)

	// Sanity check that the keys are in the collection.
	n, err := coll.Find(nil).Count()
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 2)
	for i := 0; i < 100; i++ {
		n, err := coll.Find(nil).Count()
		c.Assert(err, gc.IsNil)
		switch n {
		case 1:
			return
		case 2:
			time.Sleep(time.Second)
		default:
			c.Fatalf("unexpected key count %v", n)
		}
	}
	c.Fatalf("key was never removed from database")
}

type legacyRootKeyDoc struct {
	Id      string `bson:"_id"`
	Created time.Time
	Expires time.Time
	RootKey []byte
}

func (s *RootKeySuite) TestLegacy(c *gc.C) {
	coll := s.testColl(c)
	err := coll.Insert(&legacyRootKeyDoc{
		Id:      "foo",
		RootKey: []byte("a key"),
		Created: time.Now(),
		Expires: time.Now().Add(10 * time.Minute),
	})
	c.Assert(err, gc.IsNil)
	store := NewRootKeys(10).NewStore(coll, Policy{
		ExpiryDuration: 5 * time.Minute,
	})
	rk, err := store.Get(context.Background(), []byte("foo"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(rk), gc.Equals, "a key")
}

func (s *RootKeySuite) TestUsesSessionFromContext(c *gc.C) {
	coll := s.testColl(c)

	s1 := coll.Database.Session.Copy()
	s2 := coll.Database.Session.Copy()
	s.AddCleanup(func(c *gc.C) {
		s2.Close()
	})

	coll = coll.With(s1)
	store := NewRootKeys(10).NewStore(coll, Policy{
		ExpiryDuration: 5 * time.Minute,
	})
	s1.Close()

	ctx := ContextWithMgoSession(context.Background(), s2)
	_, _, err := store.RootKey(ctx)
	c.Assert(err, gc.Equals, nil)
}

func (s *RootKeySuite) TestDoneContext(c *gc.C) {
	store := NewRootKeys(10).NewStore(s.testColl(c), Policy{
		ExpiryDuration: 5 * time.Minute,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := store.RootKey(ctx)
	c.Assert(err, gc.ErrorMatches, `cannot query existing keys: context canceled`)
}

func (s *RootKeySuite) testColl(c *gc.C) *mgo.Collection {
	return s.Session.DB("test").C("rootkeyitems")
}

func clockVal(t *time.Time) dbrootkeystore.Clock {
	return clockFunc(func() time.Time {
		return *t
	})
}

type clockFunc func() time.Time

func (f clockFunc) Now() time.Time {
	return f()
}
