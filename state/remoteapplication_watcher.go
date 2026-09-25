// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/watcher"
)

// remoteApplicationsWatcher reports lifecycle and identity changes. Offer
// replacement and re-consumption can leave the observed lifecycle unchanged.
type remoteApplicationsWatcher struct {
	commonWatcher
	out    chan []string
	filter func(interface{}) bool
	known  map[string]remoteApplicationWatchDoc
}

type remoteApplicationWatchDoc struct {
	DocID     string `bson:"_id"`
	Life      Life   `bson:"life"`
	OfferUUID string `bson:"offer-uuid"`
	Version   int    `bson:"version"`
}

var remoteApplicationWatchFields = bson.D{
	{"_id", 1}, {"life", 1}, {"offer-uuid", 1}, {"version", 1},
}

func newRemoteApplicationsWatcher(backend modelBackend) StringsWatcher {
	w := &remoteApplicationsWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []string),
		filter:        isLocalID(backend),
		known:         make(map[string]remoteApplicationWatchDoc),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *remoteApplicationsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *remoteApplicationsWatcher) loop() error {
	in := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(remoteApplicationsC, in, w.filter)
	defer w.watcher.UnwatchCollection(remoteApplicationsC, in)
	ids, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.merge(ids, updates); err != nil {
				return err
			}
			if !ids.IsEmpty() {
				out = w.out
			}
		case out <- ids.Values():
			ids = make(set.Strings)
			out = nil
		}
	}
}

func (w *remoteApplicationsWatcher) initial() (set.Strings, error) {
	coll, closer, err := w.db.GetCollection(remoteApplicationsC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()

	ids := make(set.Strings)
	var doc remoteApplicationWatchDoc
	iter := coll.Find(nil).Select(remoteApplicationWatchFields).Iter()
	for iter.Next(&doc) {
		if !w.filter(doc.DocID) {
			continue
		}
		id := w.backend.localID(doc.DocID)
		ids.Add(id)
		if doc.Life != Dead {
			w.known[id] = doc
		}
	}
	return ids, iter.Close()
}

func (w *remoteApplicationsWatcher) merge(ids set.Strings, updates map[interface{}]bool) error {
	coll, closer, err := w.db.GetCollection(remoteApplicationsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()

	var changed []string
	latest := make(map[string]remoteApplicationWatchDoc)
	for docID, exists := range updates {
		id, ok := docID.(string)
		if !ok {
			return errors.Errorf("id is not of type string, got %T", docID)
		}
		if exists {
			changed = append(changed, id)
		} else {
			latest[w.backend.localID(id)] = remoteApplicationWatchDoc{Life: Dead}
		}
	}

	// A document removed since the update will be reported by its deletion
	// event. Compare the current identity to detect coalesced replacements.
	iter := coll.Find(bson.D{{"_id", bson.D{{"$in", changed}}}}).Select(remoteApplicationWatchFields).Iter()
	var doc remoteApplicationWatchDoc
	for iter.Next(&doc) {
		latest[w.backend.localID(doc.DocID)] = doc
	}
	if err := iter.Close(); err != nil {
		return err
	}

	for id, current := range latest {
		previous, known := w.known[id]
		switch {
		case known && current.Life == Dead:
			delete(w.known, id)
		case !known && current.Life != Dead:
			w.known[id] = current
		case known && current != previous:
			w.known[id] = current
		default:
			continue
		}
		ids.Add(id)
	}
	return nil
}
