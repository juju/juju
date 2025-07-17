// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/worker/v4"
)

// BaseWatcher represents watch methods on the worker
// responsible for watching for database changes.
type BaseWatcher interface {
	worker.Worker

	Dead() <-chan struct{}
	Err() error

	// Watch will send events on the Change channel whenever the document you
	// are watching is changed. Note that in order to not miss any changes, you
	// should start Watching the document before you read the document.
	// At this low level Watch layer, there will not be an initial event.
	// Instead, Watch is synchronous, the Watch will not return until the
	// watcher is registered.
	// TODO(jam): 2019-01-31 Update Watch() to return an error rather now
	// that it is synchronous
	Watch(collection string, id interface{}, ch chan<- Change)

	// WatchMulti is similar to Watch, it just allows you to watch a set of
	// documents in the same collection in one request. Just like Watch,
	// no event will be sent for documents that don't change.
	WatchMulti(collection string, ids []interface{}, ch chan<- Change) error

	// WatchCollection will give an event if any documents are modified/added/removed
	// from the collection.
	// TODO(jam): 2019-01-31 Update WatchCollection() to return an error rather now
	// that it is synchronous
	WatchCollection(collection string, ch chan<- Change)

	// WatchCollectionWithFilter will give an event if any documents are modified/added/removed
	// from the collection. Filter can be supplied to check if a given document
	// should send an event.
	// TODO(jam): 2019-01-31 Update WatchCollectionWithFilter() to return an error rather now
	// that it is synchronous
	WatchCollectionWithFilter(collection string, ch chan<- Change, filter func(interface{}) bool)

	// Unwatch is an asynchronous request to stop watching a given watch.
	// It is an error to try to Unwatch something that is not being watched.
	// Note that Unwatch can be called for things that have been registered with
	// either Watch() or WatchMulti(). For WatchCollection or WatchCollectionWithFilter
	// use UnwatchCollection.
	// TODO(jam): 2019-01-31 Currently Unwatching something that isn't watched
	// is a panic, should we make the method synchronous and turn it into an error?
	// Or just turn it into a no-op
	Unwatch(collection string, id interface{}, ch chan<- Change)

	// UnwatchCollection is used when you are done with a watch started with
	// either WatchCollection or WatchCollectionWithFilter. You must pass in
	// the same Change channel. Unwatching a collection that isn't being watched
	// is an error that will panic().
	UnwatchCollection(collection string, ch chan<- Change)
}

// A Change holds information about a document change.
type Change struct {
	// C and Id hold the collection name and document _id field value.
	C  string
	Id interface{}

	// Revno is the latest known value for the document's txn-revno
	// field, or -1 if the document was deleted.
	Revno int64
}

// Period is the delay between each sync.
// It must not be changed when any watchers are active.
var Period time.Duration = 5 * time.Second
