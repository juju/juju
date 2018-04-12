// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"
)

// HubSource represents the listening aspects of the pubsub hub.
type HubSource interface {
	SubscribeMatch(matcher func(string) bool, handler func(string, interface{})) func()
}

// HubWatcher listents to events from the hub and passes them on to the registered
// watchers.
type HubWatcher struct {
	hub    HubSource
	logger Logger

	tomb tomb.Tomb

	// watches holds the observers managed by Watch/Unwatch.
	watches map[watchKey][]watchInfo

	// current holds the current txn-revno values for all the observed
	// documents known to exist. Documents not observed or deleted are
	// omitted from this map and are considered to have revno -1.
	current map[watchKey]int64

	// syncEvents and requestEvents contain the events to be
	// dispatched to the watcher channels. They're queued during
	// processing and flushed at the end to simplify the algorithm.
	// The two queues are separated because events from sync are
	// handled in reverse order due to the way the algorithm works.
	syncEvents, requestEvents []event

	// request is used to deliver requests from the public API into
	// the the goroutine loop.
	request chan interface{}

	// changes are events published to the hub.
	changes chan Change
}

// NewHubWatcher returns a new watcher observing Change events published to the
// hub.
func NewHubWatcher(hub HubSource, logger Logger) *HubWatcher {
	watcher, _ := newHubWatcher(hub, logger)
	return watcher
}

func newHubWatcher(hub HubSource, logger Logger) (*HubWatcher, <-chan struct{}) {
	if logger == nil {
		logger = noOpLogger{}
	}
	started := make(chan struct{})
	w := &HubWatcher{
		hub:     hub,
		logger:  logger,
		watches: make(map[watchKey][]watchInfo),
		current: make(map[watchKey]int64),
		request: make(chan interface{}),
		changes: make(chan Change),
	}
	go func() {
		unsub := hub.SubscribeMatch(
			func(string) bool { return true }, w.receiveEvent,
		)
		defer unsub()
		close(started)
		err := w.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("watcher loop failed: %v", err)
		}
		w.tomb.Kill(cause)
		w.tomb.Done()
	}()
	return w, started
}

func (w *HubWatcher) receiveEvent(topic string, data interface{}) {
	switch topic {
	case txnWatcherStarting:
		// This message is published when the main txns.log watcher starts. If
		// this message is received here it means that the main watcher has
		// restarted. It is highly likely that it restarted because it lost
		// track of where it was, or the connection shut down. Either way, we
		// need to stop this worker to release all the watchers.
		w.tomb.Kill(errors.New("txn watcher restarted"))
	case txnWatcherCollection:
		change, ok := data.(Change)
		if !ok {
			w.logger.Warningf("incoming event not a Chage")
			return
		}
		select {
		case w.changes <- change:
		case <-w.tomb.Dying():
		}
	default:
		w.logger.Warningf("programming error, unknown topic: %q", topic)
	}
}

// Kill is part of the worker.Worker interface.
func (w *HubWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *HubWatcher) Wait() error {
	return w.tomb.Wait()
}

// Stop stops all the watcher activities.
func (w *HubWatcher) Stop() error {
	return worker.Stop(w)
}

// Dead returns a channel that is closed when the watcher has stopped.
func (w *HubWatcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

// Err returns the error with which the watcher stopped.
// It returns nil if the watcher stopped cleanly, tomb.ErrStillAlive
// if the watcher is still running properly, or the respective error
// if the watcher is terminating or has terminated with an error.
func (w *HubWatcher) Err() error {
	return w.tomb.Err()
}

func (w *HubWatcher) sendReq(req interface{}) {
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
func (w *HubWatcher) Watch(collection string, id interface{}, revno int64, ch chan<- Change) {
	if id == nil {
		panic("watcher: cannot watch a document with nil id")
	}
	w.sendReq(reqWatch{watchKey{collection, id}, watchInfo{ch, revno, nil}})
}

// WatchCollection starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection.
func (w *HubWatcher) WatchCollection(collection string, ch chan<- Change) {
	w.WatchCollectionWithFilter(collection, ch, nil)
}

// WatchCollectionWithFilter starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection, so long as the
// specified filter function returns true when called with the document id value.
func (w *HubWatcher) WatchCollectionWithFilter(collection string, ch chan<- Change, filter func(interface{}) bool) {
	w.sendReq(reqWatch{watchKey{collection, nil}, watchInfo{ch, 0, filter}})
}

// Unwatch stops watching the given collection and document id via ch.
func (w *HubWatcher) Unwatch(collection string, id interface{}, ch chan<- Change) {
	if id == nil {
		panic("watcher: cannot unwatch a document with nil id")
	}
	w.sendReq(reqUnwatch{watchKey{collection, id}, ch})
}

// UnwatchCollection stops watching the given collection via ch.
func (w *HubWatcher) UnwatchCollection(collection string, ch chan<- Change) {
	w.sendReq(reqUnwatch{watchKey{collection, nil}, ch})
}

// loop implements the main watcher loop.
// period is the delay between each sync.
func (w *HubWatcher) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case change := <-w.changes:
			w.queueChange(change)
		case req := <-w.request:
			w.handle(req)
		}
		for (len(w.syncEvents) + len(w.requestEvents)) > 0 {
			w.flush()
		}
	}
}

func (w *HubWatcher) flush() {
	// syncEvents are stored first in first out.
	// syncEvents may grow during the looping here if new
	// watch events come in while we are notifying other watchers.
	for i := 0; i < len(w.syncEvents); i++ {
		// We need to reget the address value each time through the loop
		// as the slice may be reallocated.
		for e := &w.syncEvents[i]; e.ch != nil; e = &w.syncEvents[i] {
			w.logger.Tracef("syncEvents: e.ch=%v len(%d), cap(%d)", e.ch, len(w.syncEvents), cap(w.syncEvents))
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case change := <-w.changes:
				w.queueChange(change)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
				w.logger.Tracef("e.ch=%v has been notified", e.ch)
			}
			break
		}
	}
	w.syncEvents = w.syncEvents[:0]
	w.logger.Tracef("syncEvents: len(%d), cap(%d)", len(w.syncEvents), cap(w.syncEvents))

	// requestEvents are stored oldest first, and
	// may grow during the loop.
	for i := 0; i < len(w.requestEvents); i++ {
		// We need to reget the address value each time through the loop
		// as the slice may be reallocated.
		for e := &w.requestEvents[i]; e.ch != nil; e = &w.requestEvents[i] {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case change := <-w.changes:
				w.queueChange(change)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
			}
			break
		}
	}
	w.requestEvents = w.requestEvents[:0]
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *HubWatcher) handle(req interface{}) {
	w.logger.Tracef("got request: %#v", req)
	switch r := req.(type) {
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

// queueChange queues up the change for the registered watchers.
func (w *HubWatcher) queueChange(change Change) {
	w.logger.Tracef("got change document: %#v", change)
	key := watchKey{change.C, change.Id}
	revno := change.Revno
	w.current[key] = revno

	// Queue notifications for per-collection watches.
	for _, info := range w.watches[watchKey{change.C, nil}] {
		if info.filter != nil && !info.filter(change.Id) {
			continue
		}
		w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
		w.logger.Tracef("adding collection watch for %v syncEvents: len(%d), cap(%d)", info.ch, len(w.syncEvents), cap(w.syncEvents))
	}

	// Queue notifications for per-document watches.
	infos := w.watches[key]
	for i, info := range infos {
		if revno > info.revno || revno < 0 && info.revno >= 0 {
			infos[i].revno = revno
			w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
			w.logger.Tracef("adding document watch for %v syncEvents: len(%d), cap(%d)", info.ch, len(w.syncEvents), cap(w.syncEvents))
		}
	}
}
