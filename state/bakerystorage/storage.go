// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/mgo.v2"
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
	rootKeys    *mgostorage.RootKeys
}

type storageDoc struct {
	Location string    `bson:"_id"`
	Item     string    `bson:"item"`
	ExpireAt time.Time `bson:"expire-at,omitempty"`
}

// ExpireAfter implements ExpirableStorage.ExpireAfter.
func (s *storage) ExpireAfter(expireAfter time.Duration) ExpirableStorage {
	newStorage := *s
	newStorage.expireAfter = expireAfter
	return &newStorage
}

func (s *storage) RootKey() ([]byte, []byte, error) {
	storage, closer := s.getStorage()
	defer closer()
	return storage.RootKey()
}

func (s *storage) getStorage() (bakery.Storage, func()) {
	coll, closer := s.config.GetCollection()
	return s.config.GetStorage(s.rootKeys, coll, s.expireAfter), closer
}

func (s *storage) Get(id []byte) ([]byte, error) {
	storage, closer := s.getStorage()
	defer closer()
	i, err := storage.Get(id)
	if err != nil {
		return s.legacyGet(id)
	}
	return i, nil
}

func (s *storage) legacyGet(location []byte) ([]byte, error) {
	coll, closer := s.config.GetLegacyCollection()
	defer closer()

	var i storageDoc
	err := coll.FindId(string(location)).One(&i)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, bakery.ErrNotFound
		}
		return nil, errors.Annotatef(err, "cannot get item for location %q", location)
	}
	return []byte(i.Item), nil
}
