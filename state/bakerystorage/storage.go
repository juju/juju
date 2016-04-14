// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"
)

var expiryTimeIndex = mgo.Index{
	Key:    []string{"expire-at"},
	Sparse: true,

	// We expire records when the clock time is one
	// second older than the record's expire-at field
	// value. It has to be at least one second, because
	// mgo uses "omitempty" for this field.
	ExpireAfter: time.Second,
}

type storage struct {
	config   Config
	expireAt time.Time
}

type storageDoc struct {
	Location string    `bson:"_id"`
	Item     string    `bson:"item"`
	ExpireAt time.Time `bson:"expire-at,omitempty"`
}

// ExpireAt implements ExpirableStorage.ExpireAt.
func (s *storage) ExpireAt(expireAt time.Time) ExpirableStorage {
	return &storage{s.config, expireAt}
}

// Put implements bakery.Storage.Put.
func (s *storage) Put(location, item string) error {
	coll, closer := s.config.GetCollection()
	defer closer()

	doc := storageDoc{
		Location: location,
		Item:     item,
	}
	if !s.expireAt.IsZero() {
		// NOTE(axw) we subtract one second from the expiry time, because
		// the expireAfterSeconds index we create is 1 and not 0 due to
		// a limitation in the mgo EnsureIndex API.
		doc.ExpireAt = s.expireAt.Add(-1 * time.Second)
	}
	_, err := coll.Writeable().UpsertId(location, doc)
	if err != nil {
		return errors.Annotatef(err, "cannot store item for location %q", location)
	}
	return nil
}

// Get implements bakery.Storage.Get.
func (s *storage) Get(location string) (string, error) {
	coll, closer := s.config.GetCollection()
	defer closer()

	var i storageDoc
	err := coll.FindId(location).One(&i)
	if err != nil {
		if err == mgo.ErrNotFound {
			return "", bakery.ErrNotFound
		}
		return "", errors.Annotatef(err, "cannot get item for location %q", location)
	}
	return i.Item, nil
}

// Del implements bakery.Storage.Del.
func (s *storage) Del(location string) error {
	coll, closer := s.config.GetCollection()
	defer closer()

	err := coll.Writeable().RemoveId(location)
	if err != nil {
		if err == mgo.ErrNotFound {
			// Not an error to remove an item that doesn't exist.
			return nil
		}
		return errors.Annotatef(err, "cannot remove item for location %q", location)
	}
	return nil
}
