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

// Collection imperfectly insulates clients from the capacity to write to
// MongoDB. Query results can still be used to write; and the Writeable
// method exposes the underlying *mgo.Collection when absolutely required;
// but the general expectation in juju is that writes will occur only via
// mgo/txn, and any layer-skipping is done only in exceptional and well-
// supported circumstances.
type Collection interface {

	// Name returns the name of the collection.
	Name() string

	// Count, Find, and FindId methods act as documented for *mgo.Collection.
	Count() (int, error)
	Find(query interface{}) *mgo.Query
	FindId(id interface{}) *mgo.Query

	// Writeable gives access to methods that enable direct DB access. It
	// should be used with judicious care, and for only the best of reasons.
	Writeable() WriteCollection
}

// WriteCollection allows read/write access to a MongoDB collection.
type WriteCollection interface {
	Collection

	// Underlying returns the underlying *mgo.Collection.
	Underlying() *mgo.Collection

	// All other methods act as documented for *mgo.Collection.
	Insert(docs ...interface{}) error
	Upsert(selector interface{}, update interface{}) (info *mgo.ChangeInfo, err error)
	UpsertId(id interface{}, update interface{}) (info *mgo.ChangeInfo, err error)
	Update(selector interface{}, update interface{}) error
	UpdateId(id interface{}, update interface{}) error
	Remove(sel interface{}) error
	RemoveId(id interface{}) error
	RemoveAll(sel interface{}) (*mgo.ChangeInfo, error)
}

// WrapCollection returns a Collection that wraps the supplied *mgo.Collection.
func WrapCollection(coll *mgo.Collection) Collection {
	return collectionWrapper{coll}
}

// collectionWrapper wraps a *mgo.Collection and implements Collection and
// WriteCollection.
type collectionWrapper struct {
	*mgo.Collection
}

// Name is part of the Collection interface.
func (cw collectionWrapper) Name() string {
	return cw.Collection.Name
}

// Writeable is part of the Collection interface.
func (cw collectionWrapper) Writeable() WriteCollection {
	return cw
}

// Underlying is part of the WriteCollection interface.
func (cw collectionWrapper) Underlying() *mgo.Collection {
	return cw.Collection
}
