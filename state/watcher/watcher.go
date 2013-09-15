// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The watcher package provides an interface for observing changes
// to arbitrary MongoDB documents that are maintained via the
// mgo/txn transaction package.
package watcher

import (
	"fmt"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/log"
)

// Debug specifies whether the package will log debug
// messages.
// TODO(rog) allow debug level setting in the log package.
var Debug = false

// A Watcher can watch any number of collections and documents for changes.
type Watcher struct {
	tomb tomb.Tomb
	log  *mgo.Collection

	// watches holds the observers managed by Watch/Unwatch.
	watches map[watchKey][]watchInfo

	// current holds the current txn-revno values for all the observed
	// documents known to exist. Documents not observed or deleted are
	// omitted from this map and are considered to have revno -1.
	current map[watchKey]int64

	// needSync is set when a synchronization should take
	// place.
	needSync bool

	// syncEvents and requestEvents contain the events to be
	// dispatched to the watcher channels. They're queued during
	// processing and flushed at the end to simplify the algorithm.
	// The two queues are separated because events from sync are
	// handled in reverse order due to the way the algorithm works.
	syncEvents, requestEvents []event

	// request is used to deliver requests from the public API into
	// the the goroutine loop.
	request chan interface{}

	// lastId is the most recent transaction id observed by a sync.
	lastId interface{}
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

type watchKey struct {
	c  string
	id interface{} // nil when watching collection
}

func (k watchKey) String() string {
	coll := "collection " + k.c
	if k.id == nil {
		return coll
	}
	return fmt.Sprintf("document %v in %s", k.id, coll)
}

// match returns whether the receiving watch key,
// which may refer to a particular item or
// an entire collection, matches k1, which refers
// to a particular item.
func (k watchKey) match(k1 watchKey) bool {
	if k.c != k1.c {
		return false
	}
	if k.id == nil {
		// k refers to entire collection
		return true
	}
	return k.id == k1.id
}

type watchInfo struct {
	ch     chan<- Change
	revno  int64
	filter func(interface{}) bool
}

type event struct {
	ch    chan<- Change
	key   watchKey
	revno int64
}

// New returns a new Watcher observing the changelog collection,
// which must be a capped collection maintained by mgo/txn.
func New(changelog *mgo.Collection) *Watcher {
	w := &Watcher{
		log:     changelog,
		watches: make(map[watchKey][]watchInfo),
		current: make(map[watchKey]int64),
		request: make(chan interface{}),
	}
	go func() {
		w.tomb.Kill(w.loop())
		w.tomb.Done()
	}()
	return w
}

// Stop stops all the watcher activities.
func (w *Watcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Dead returns a channel that is closed when the watcher has stopped.
func (w *Watcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

// Err returns the error with which the watcher stopped.
// It returns nil if the watcher stopped cleanly, tomb.ErrStillAlive
// if the watcher is still running properly, or the respective error
// if the watcher is terminating or has terminated with an error.
func (w *Watcher) Err() error {
	return w.tomb.Err()
}

type reqWatch struct {
	key  watchKey
	info watchInfo
}

type reqUnwatch struct {
	key watchKey
	ch  chan<- Change
}

type reqSync struct{}

func (w *Watcher) sendReq(req interface{}) {
	select {
	case w.request <- req:
	case <-w.tomb.Dying():
	}
}

// Watch starts watching the given collection and document id.
// An event will be sent onto ch whenever a matching document's txn-revno
// field is observed to change after a transaction is applied. The revno
// parameter holds the currently known revision number for the document.
// Non-existent documents are represented by a -1 revno.
func (w *Watcher) Watch(collection string, id interface{}, revno int64, ch chan<- Change) {
	if id == nil {
		panic("watcher: cannot watch a document with nil id")
	}
	w.sendReq(reqWatch{watchKey{collection, id}, watchInfo{ch, revno, nil}})
}

// WatchCollection starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection.
func (w *Watcher) WatchCollection(collection string, ch chan<- Change) {
	w.WatchCollectionWithFilter(collection, ch, nil)
}

// WatchCollectionWithFilter starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection, so long as the
// specified filter function returns true when called with the document id value.
func (w *Watcher) WatchCollectionWithFilter(collection string, ch chan<- Change, filter func(interface{}) bool) {
	w.sendReq(reqWatch{watchKey{collection, nil}, watchInfo{ch, 0, filter}})
}

// Unwatch stops watching the given collection and document id via ch.
func (w *Watcher) Unwatch(collection string, id interface{}, ch chan<- Change) {
	if id == nil {
		panic("watcher: cannot unwatch a document with nil id")
	}
	w.sendReq(reqUnwatch{watchKey{collection, id}, ch})
}

// UnwatchCollection stops watching the given collection via ch.
func (w *Watcher) UnwatchCollection(collection string, ch chan<- Change) {
	w.sendReq(reqUnwatch{watchKey{collection, nil}, ch})
}

// StartSync forces the watcher to load new events from the database.
func (w *Watcher) StartSync() {
	w.sendReq(reqSync{})
}

// Period is the delay between each sync.
// It must not be changed when any watchers are active.
var Period time.Duration = 5 * time.Second

// loop implements the main watcher loop.
func (w *Watcher) loop() error {
	next := time.After(Period)
	w.needSync = true
	if err := w.initLastId(); err != nil {
		return err
	}
	for {
		if w.needSync {
			if err := w.sync(); err != nil {
				return err
			}
			w.flush()
			next = time.After(Period)
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-next:
			next = time.After(Period)
			w.needSync = true
		case req := <-w.request:
			w.handle(req)
			w.flush()
		}
	}
}

// flush sends all pending events to their respective channels.
func (w *Watcher) flush() {
	// refreshEvents are stored newest first.
	for i := len(w.syncEvents) - 1; i >= 0; i-- {
		e := &w.syncEvents[i]
		for e.ch != nil {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
			}
			break
		}
	}
	// requestEvents are stored oldest first, and
	// may grow during the loop.
	for i := 0; i < len(w.requestEvents); i++ {
		e := &w.requestEvents[i]
		for e.ch != nil {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
			}
			break
		}
	}
	w.syncEvents = w.syncEvents[:0]
	w.requestEvents = w.requestEvents[:0]
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *Watcher) handle(req interface{}) {
	debugf("state/watcher: got request: %#v", req)
	switch r := req.(type) {
	case reqSync:
		w.needSync = true
	case reqWatch:
		for _, info := range w.watches[r.key] {
			if info.ch == r.info.ch {
				panic(fmt.Errorf("tried to re-add channel %v for %s", info.ch, r.key))
			}
		}
		if revno, ok := w.current[r.key]; ok && (revno > r.info.revno || revno == -1 && r.info.revno >= 0) {
			r.info.revno = revno
			w.requestEvents = append(w.requestEvents, event{r.info.ch, r.key, revno})
		}
		w.watches[r.key] = append(w.watches[r.key], r.info)
	case reqUnwatch:
		watches := w.watches[r.key]
		removed := false
		for i, info := range watches {
			if info.ch == r.ch {
				watches[i] = watches[len(watches)-1]
				w.watches[r.key] = watches[:len(watches)-1]
				removed = true
				break
			}
		}
		if !removed {
			panic(fmt.Errorf("tried to remove missing channel %v for %s", r.ch, r.key))
		}
		for i := range w.requestEvents {
			e := &w.requestEvents[i]
			if r.key.match(e.key) && e.ch == r.ch {
				e.ch = nil
			}
		}
		for i := range w.syncEvents {
			e := &w.syncEvents[i]
			if r.key.match(e.key) && e.ch == r.ch {
				e.ch = nil
			}
		}
	default:
		panic(fmt.Errorf("unknown request: %T", req))
	}
}

type logInfo struct {
	Docs   []interface{} `bson:"d"`
	Revnos []int64       `bson:"r"`
}

// initLastId reads the most recent changelog document and initializes
// lastId with it. This causes all history that precedes the creation
// of the watcher to be ignored.
func (w *Watcher) initLastId() error {
	var entry struct {
		Id interface{} "_id"
	}
	err := w.log.Find(nil).Sort("-$natural").One(&entry)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	w.lastId = entry.Id
	return nil
}

// sync updates the watcher knowledge from the database, and
// queues events to observing channels.
func (w *Watcher) sync() error {
	w.needSync = false
	// Iterate through log events in reverse insertion order (newest first).
	iter := w.log.Find(nil).Batch(10).Sort("-$natural").Iter()
	seen := make(map[watchKey]bool)
	first := true
	lastId := w.lastId
	var entry bson.D
	for iter.Next(&entry) {
		if len(entry) == 0 {
			debugf("state/watcher: got empty changelog document")
		}
		id := entry[0]
		if id.Name != "_id" {
			panic("watcher: _id field isn't first entry")
		}
		if first {
			w.lastId = id.Value
			first = false
		}
		if id.Value == lastId {
			break
		}
		debugf("state/watcher: got changelog document: %#v", entry)
		for _, c := range entry[1:] {
			// See txn's Runner.ChangeLog for the structure of log entries.
			var d, r []interface{}
			dr, _ := c.Value.(bson.D)
			for _, item := range dr {
				switch item.Name {
				case "d":
					d, _ = item.Value.([]interface{})
				case "r":
					r, _ = item.Value.([]interface{})
				}
			}
			if len(d) == 0 || len(d) != len(r) {
				log.Warningf("state/watcher: changelog has invalid collection document: %#v", c)
				continue
			}
			for i := len(d) - 1; i >= 0; i-- {
				key := watchKey{c.Name, d[i]}
				if seen[key] {
					continue
				}
				seen[key] = true
				revno, ok := r[i].(int64)
				if !ok {
					log.Warningf("state/watcher: changelog has revno with type %T: %#v", r[i], r[i])
					continue
				}
				if revno < 0 {
					revno = -1
				}
				if w.current[key] == revno {
					continue
				}
				w.current[key] = revno
				// Queue notifications for per-collection watches.
				for _, info := range w.watches[watchKey{c.Name, nil}] {
					if info.filter != nil && !info.filter(d[i]) {
						continue
					}
					w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
				}
				// Queue notifications for per-document watches.
				infos := w.watches[key]
				for i, info := range infos {
					if revno > info.revno || revno < 0 && info.revno >= 0 {
						infos[i].revno = revno
						w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
					}
				}
			}
		}
	}
	if iter.Err() != nil {
		return fmt.Errorf("watcher iteration error: %v", iter.Err())
	}
	return nil
}

func debugf(f string, a ...interface{}) {
	if Debug {
		log.Debugf(f, a...)
	}
}
