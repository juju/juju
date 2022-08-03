// Copyright 2014-2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
)

// Functions defined as variables so they can be overidden
// for testing.
var (
	clock               dbrootkeystore.Clock
	mgoCollectionFindId = (*mgo.Collection).FindId
)

// TODO it would be nice if could make Policy
// a type alias for dbrootkeystore.Policy,
// but we want to be able to support versions
// of Go from before type aliases were introduced.

// Policy holds a store policy for root keys.
type Policy dbrootkeystore.Policy

// RootKeys represents a cache of macaroon root keys.
type RootKeys struct {
	keys *dbrootkeystore.RootKeys
}

// NewRootKeys returns a root-keys cache that
// is limited in size to approximately the given size.
//
// The NewStore method returns a store implementation
// that uses a specific mongo collection and store
// policy.
func NewRootKeys(maxCacheSize int) *RootKeys {
	return &RootKeys{
		keys: dbrootkeystore.NewRootKeys(maxCacheSize, clock),
	}
}

// NewStore returns a new RootKeyStore implementation that
// stores and obtains root keys from the given collection.
//
// Root keys will be generated and stored following the
// given store policy.
//
// It is expected that all collections passed to a given Store's
// NewStore method should refer to the same underlying collection.
func (s *RootKeys) NewStore(c *mgo.Collection, policy Policy) bakery.RootKeyStore {
	return s.keys.NewStore(backing{c}, dbrootkeystore.Policy(policy))
}

var indexes = []mgo.Index{{
	Key: []string{"-created"},
}, {
	Key:         []string{"expires"},
	ExpireAfter: time.Second,
}}

// EnsureIndex ensures that the required indexes exist on the
// collection that will be used for root key store.
// This should be called at least once before using NewStore.
func (s *RootKeys) EnsureIndex(c *mgo.Collection) error {
	for _, idx := range indexes {
		if err := c.EnsureIndex(idx); err != nil {
			return errors.Annotatef(err, "cannot ensure index for %q on %q", idx.Key, c.Name)
		}
	}
	return nil
}

type backing struct {
	coll *mgo.Collection
}

var _ dbrootkeystore.Backing = backing{}
var _ dbrootkeystore.ContextBacking = backing{}

// GetKey implements dbrootkeystore.Backing.
func (b backing) GetKey(id []byte) (dbrootkeystore.RootKey, error) {
	return getFromMongo(b.coll, id)
}

// GetKeyContext implements dbrootkeystore.ContextBacking.
func (b backing) GetKeyContext(ctx context.Context, id []byte) (dbrootkeystore.RootKey, error) {
	var rk dbrootkeystore.RootKey
	var err error

	f := func(coll *mgo.Collection) {
		rk, err = getFromMongo(coll, id)
	}

	if err := b.runWithContext(ctx, f); err != nil {
		return dbrootkeystore.RootKey{}, err
	}
	return rk, err
}

func getFromMongo(coll *mgo.Collection, id []byte) (dbrootkeystore.RootKey, error) {
	var key dbrootkeystore.RootKey
	err := mgoCollectionFindId(coll, id).One(&key)
	if err != nil {
		if err == mgo.ErrNotFound {
			return getLegacyFromMongo(coll, string(id))
		}
		return dbrootkeystore.RootKey{}, errors.Annotatef(err, "cannot get key from database")
	}
	// TODO migrate the key from the old format to the new format.
	return key, nil
}

// getLegacyFromMongo gets a value from the old version of the
// root key document which used a string key rather than a []byte
// key.
func getLegacyFromMongo(coll *mgo.Collection, id string) (dbrootkeystore.RootKey, error) {
	var key dbrootkeystore.RootKey
	err := mgoCollectionFindId(coll, id).One(&key)
	if err != nil {
		if err == mgo.ErrNotFound {
			return dbrootkeystore.RootKey{}, bakery.ErrNotFound
		}
		return dbrootkeystore.RootKey{}, errors.Annotatef(err, "cannot get key from database")
	}
	return key, nil
}

// FindLatestKey implements dbrootkeystore.Backing.
func (b backing) FindLatestKey(createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	return findLatestKey(b.coll, createdAfter, expiresAfter, expiresBefore)
}

// FindLatestKeyContext implements dbrootkeystore.ContextBacking.
func (b backing) FindLatestKeyContext(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	var rk dbrootkeystore.RootKey
	var err error

	f := func(coll *mgo.Collection) {
		rk, err = findLatestKey(coll, createdAfter, expiresAfter, expiresBefore)
	}

	if err := b.runWithContext(ctx, f); err != nil {
		return dbrootkeystore.RootKey{}, err
	}

	return rk, err
}

func findLatestKey(coll *mgo.Collection, createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	var key dbrootkeystore.RootKey
	err := coll.Find(bson.D{{
		"created", bson.D{{"$gte", createdAfter}},
	}, {
		"expires", bson.D{
			{"$gte", expiresAfter},
			{"$lte", expiresBefore},
		},
	}}).Sort("-created").One(&key)
	if err != nil && err != mgo.ErrNotFound {
		return dbrootkeystore.RootKey{}, errors.Annotatef(err, "cannot query existing keys")
	}
	return key, nil
}

// InsertKey implements dbrootkeystore.Backing.
func (b backing) InsertKey(key dbrootkeystore.RootKey) error {
	return insertKey(b.coll, key)
}

// InsertKeyContext implements dbrootkeystore.ContextBacking.
func (b backing) InsertKeyContext(ctx context.Context, key dbrootkeystore.RootKey) error {
	var err error

	f := func(coll *mgo.Collection) {
		err = insertKey(coll, key)
	}

	if err := b.runWithContext(ctx, f); err != nil {
		return err
	}

	return err
}

func insertKey(coll *mgo.Collection, key dbrootkeystore.RootKey) error {
	if err := coll.Insert(key); err != nil {
		return errors.Annotatef(err, "mongo insert failed")
	}
	return nil
}

func (b backing) runWithContext(ctx context.Context, f func(*mgo.Collection)) error {
	s := sessionFromContext(ctx)
	if s == nil {
		s = b.coll.Database.Session
	}
	s = s.Clone()

	c := make(chan struct{})

	go func() {
		defer close(c)
		defer s.Close()
		f(b.coll.With(s))
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c:
		return nil
	}
}

type contextSessionKey struct{}

// ContextWithMgoSession adds the given mgo.Session to the given
// context.Context. Any operations requiring database access that are
// made using a context with an attached session will use the session
// from the context to access mongodb, rather than the session in the
// collection used when the RootKeyStore was created.
func ContextWithMgoSession(ctx context.Context, s *mgo.Session) context.Context {
	return context.WithValue(ctx, contextSessionKey{}, s)
}

func sessionFromContext(ctx context.Context) *mgo.Session {
	s, _ := ctx.Value(contextSessionKey{}).(*mgo.Session)
	return s
}
