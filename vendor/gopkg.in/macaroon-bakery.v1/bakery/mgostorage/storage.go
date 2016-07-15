// Package mgostorage provides an implementation of the
// bakery Storage interface that uses MongoDB to store
// items.
package mgostorage

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"gopkg.in/macaroon-bakery.v1/bakery"
)

// New returns an implementation of Storage
// that stores all items in MongoDB.
// It never returns an error (the error return
// is for backward compatibility with a previous
// version that could return an error).
//
// Note that the caller is responsible for closing
// the mgo session associated with the collection.
func New(c *mgo.Collection) (bakery.Storage, error) {
	return mgoStorage{
		col: c,
	}, nil
}

type mgoStorage struct {
	col *mgo.Collection
}

type storageDoc struct {
	Location string `bson:"_id"`
	Item     string `bson:"item"`

	// OldLocation is set for backward compatibility reasons - the
	// original version of the code used "loc" as a unique index
	// so we need to maintain the uniqueness otherwise
	// inserts will fail.
	// TODO remove this when moving to bakery.v2.
	OldLocation string `bson:"loc"`
}

// Put implements bakery.Storage.Put.
func (s mgoStorage) Put(location, item string) error {
	i := storageDoc{
		Location:    location,
		OldLocation: location,
		Item:        item,
	}
	_, err := s.col.UpsertId(location, i)
	if err != nil {
		return errgo.Notef(err, "cannot store item for location %q", location)
	}
	return nil
}

// Get implements bakery.Storage.Get.
func (s mgoStorage) Get(location string) (string, error) {
	var i storageDoc
	err := s.col.FindId(location).One(&i)
	if err != nil {
		if err == mgo.ErrNotFound {
			return "", bakery.ErrNotFound
		}
		return "", errgo.Notef(err, "cannot get %q", location)
	}

	return i.Item, nil
}

// Del implements bakery.Storage.Del.
func (s mgoStorage) Del(location string) error {
	err := s.col.RemoveId(location)
	if err != nil {
		return errgo.Notef(err, "cannot remove %q", location)
	}
	return nil
}
