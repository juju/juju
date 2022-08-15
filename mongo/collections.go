// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"time"

	"github.com/juju/mgo/v3"
)

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
	Find(query interface{}) Query
	FindId(id interface{}) Query
	Pipe(pipeline interface{}) *mgo.Pipe

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

// Query allows access to a portion of a MongoDB collection.
type Query interface {
	All(result interface{}) error
	Apply(change mgo.Change, result interface{}) (info *mgo.ChangeInfo, err error)
	Batch(n int) Query
	Comment(comment string) Query
	Count() (n int, err error)
	Distinct(key string, result interface{}) error
	Explain(result interface{}) error
	For(result interface{}, f func() error) error
	Hint(indexKey ...string) Query
	Iter() Iterator
	Limit(n int) Query
	LogReplay() Query
	MapReduce(job *mgo.MapReduce, result interface{}) (info *mgo.MapReduceInfo, err error)
	One(result interface{}) (err error)
	Prefetch(p float64) Query
	Select(selector interface{}) Query
	SetMaxScan(n int) Query
	SetMaxTime(d time.Duration) Query
	Skip(n int) Query
	Snapshot() Query
	Sort(fields ...string) Query
	Tail(timeout time.Duration) *mgo.Iter
}

// Iterator defines the parts of the mgo.Iter that we use - this
// interface allows us to switch out the querying for testing.
type Iterator interface {
	Next(interface{}) bool
	Timeout() bool
	Close() error
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

// Find is part of the Collection interface.
func (cw collectionWrapper) Find(query interface{}) Query {
	return queryWrapper{cw.Collection.Find(query)}
}

// Pipe is part of the Collection interface
func (cw collectionWrapper) Pipe(pipeline interface{}) *mgo.Pipe {
	return cw.Collection.Pipe(pipeline)
}

// FindId is part of the Collection interface.
func (cw collectionWrapper) FindId(id interface{}) Query {
	return queryWrapper{cw.Collection.FindId(id)}
}

// Writeable is part of the Collection interface.
func (cw collectionWrapper) Writeable() WriteCollection {
	return cw
}

// Underlying is part of the WriteCollection interface.
func (cw collectionWrapper) Underlying() *mgo.Collection {
	return cw.Collection
}

type queryWrapper struct {
	*mgo.Query
}

func (qw queryWrapper) Batch(n int) Query {
	return queryWrapper{qw.Query.Batch(n)}
}

func (qw queryWrapper) Comment(comment string) Query {
	return queryWrapper{qw.Query.Comment(comment)}
}

func (qw queryWrapper) Hint(indexKey ...string) Query {
	return queryWrapper{qw.Query.Hint(indexKey...)}
}

func (qw queryWrapper) Limit(n int) Query {
	return queryWrapper{qw.Query.Limit(n)}
}

func (qw queryWrapper) LogReplay() Query {
	return queryWrapper{qw.Query.LogReplay()}
}

func (qw queryWrapper) Prefetch(p float64) Query {
	return queryWrapper{qw.Query.Prefetch(p)}
}

func (qw queryWrapper) Select(selector interface{}) Query {
	return queryWrapper{qw.Query.Select(selector)}
}

func (qw queryWrapper) SetMaxScan(n int) Query {
	return queryWrapper{qw.Query.SetMaxScan(n)}
}

func (qw queryWrapper) SetMaxTime(d time.Duration) Query {
	return queryWrapper{qw.Query.SetMaxTime(d)}
}

func (qw queryWrapper) Skip(n int) Query {
	return queryWrapper{qw.Query.Skip(n)}
}

func (qw queryWrapper) Snapshot() Query {
	return queryWrapper{qw.Query.Snapshot()}
}

func (qw queryWrapper) Sort(fields ...string) Query {
	return queryWrapper{qw.Query.Sort(fields...)}
}

func (qw queryWrapper) Iter() Iterator {
	return qw.Query.Iter()
}
