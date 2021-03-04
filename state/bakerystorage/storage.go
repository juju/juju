// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/mgorootkeystore"
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
)

// MongoIndexes returns the indexes to apply to the MongoDB collection.
func MongoIndexes() []mgo.Index {
	// Note: this second-guesses the underlying document format
	// used by bakery's mgostorage package.
	// TODO change things so that we can use EnsureIndex instead.
	return []mgo.Index{{
		Key: []string{"-created"},
	}, {
		Key:         []string{"expires"},
		ExpireAfter: time.Second,
	}}
}

type storage struct {
	config      Config
	expireAfter time.Duration
	rootKeys    *mgorootkeystore.RootKeys
}

type storageDoc struct {
	Location string    `bson:"_id"`
	Item     string    `bson:"item"`
	ExpireAt time.Time `bson:"expire-at,omitempty"`
}

type legacyRootKey struct {
	RootKey []byte
}

// ExpireAfter implements ExpirableStorage.ExpireAfter.
func (s *storage) ExpireAfter(expireAfter time.Duration) ExpirableStorage {
	newStorage := *s
	newStorage.expireAfter = expireAfter
	return &newStorage
}

// RootKey implements Storage.RootKey
func (s *storage) RootKey(ctx context.Context) ([]byte, []byte, error) {
	storage, closer := s.getStorage()
	defer closer()
	return storage.RootKey(ctx)
}

func (s *storage) getStorage() (bakery.RootKeyStore, func()) {
	coll, closer := s.config.GetCollection()
	return s.config.GetStorage(s.rootKeys, coll, s.expireAfter), closer
}

// Get implements Storage.Get
func (s *storage) Get(ctx context.Context, id []byte) ([]byte, error) {
	storage, closer := s.getStorage()
	defer closer()
	i, err := storage.Get(ctx, id)
	if err != nil {
		if err == bakery.ErrNotFound {
			return s.legacyGet(id)
		}
		return nil, err
	}
	return i, nil
}

// legacyGet is attempted as the id we're looking for was created in a previous
// version of Juju while using v1 versions of the macaroon-bakery.
func (s *storage) legacyGet(location []byte) ([]byte, error) {
	coll, closer := s.config.GetCollection()
	defer closer()

	var i storageDoc
	err := coll.FindId(string(location)).One(&i)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, bakery.ErrNotFound
		}
		return nil, errors.Annotatef(err, "cannot get item for location %q", location)
	}
	var rootKey legacyRootKey
	err = json.Unmarshal([]byte(i.Item), &rootKey)
	if err != nil {
		return nil, errors.Annotate(err, "was unable to unmarshal found rootkey")
	}
	return rootKey.RootKey, nil
}
