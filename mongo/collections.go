// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import "gopkg.in/mgo.v2"

// CollectionFromName returns a named collection on the specified database,
// initialised with a new session. Also returned is a close function which
// must be called when the collection is no longer required.
func CollectionFromName(db *mgo.Database, coll string) (Collection, func()) {
	session := db.Session.Copy()
	newColl := db.C(coll).With(session)
	return WrapCollection(newColl), session.Close
}

// Collection allows us to construct wrappers for *mgo.Collection objects.
// It's not generally useful for mocking, because (1) you need a real mongo
// in the background to implement Find[Id], not to mention Underlying; and
// (2) if you're working at the level where you're directly concerned with
// collections, it's important to write tests that verify their practical
// behaviour.
type Collection interface {

	// Name returns the name of the collection.
	Name() string

	// Underlying returns the underlying *mgo.Collection. If you're using
	// the collection with mgo/txn, you should be very wary of this method:
	// careless use will disrupt the operation of mgo/txn and cause arbitrary
	// badness.
	Underlying() *mgo.Collection

	// All other methods act as documented for *mgo.Collection.
	Count() (int, error)
	Find(query interface{}) *mgo.Query
	FindId(id interface{}) *mgo.Query
}

// WrapCollection returns a Collection that wraps the supplied *mgo.Collection.
func WrapCollection(coll *mgo.Collection) Collection {
	return collectionWrapper{coll}
}

// collectionWrapper wraps a *mgo.Collection and implements Collection.
type collectionWrapper struct {
	*mgo.Collection
}

// Name is part of the Collection interface.
func (cw collectionWrapper) Name() string {
	return cw.Collection.Name
}

// Underlying is part of the Collection interface.
func (cw collectionWrapper) Underlying() *mgo.Collection {
	return cw.Collection
}
