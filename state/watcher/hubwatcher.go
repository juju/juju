// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"fmt"
	"reflect"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"
)

var (
	// HubWatcherIdleFunc allows tets to be able to get callbacks
	// when the hub watcher hasn't notified any watchers for a specified time.
	HubWatcherIdleFunc func(string)

	// HubWatcherIdleTime relates to how long the hub needs to wait
	// having notified no watchers to be considered idle.
	HubWatcherIdleTime = 50 * time.Millisecond
)

// HubSource represents the listening aspects of the pubsub hub.
type HubSource interface {
	SubscribeMatch(matcher func(string) bool, handler func(string, interface{})) func()
}

// filterFactor represents the portion of a first-order filter that is calculating a running average.
// This is the weight of the new inputs. A value of 0.5 would represent averaging over just 2 samples,
// 0.1 would be 10 samples, 0.01 corresponds to averaging over approximately 100 items.
const filterFactor = 0.01

// HubWatcher listens to events from the hub and passes them on to the registered
// watchers.
type HubWatcher struct {
	hub       HubSource
	clock     Clock
	modelUUID string
	idleFunc  func(string)
	logger    Logger

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

	// lastSyncLen was the length of syncEvents in the last flush()
	lastSyncLen int

	// maxSyncLen is the longest we've seen syncEvents
	maxSyncLen int

	// averageSyncLen applies a first-order filter for every time we flush(), to give us an idea of how large,
	// on average, our sync queue is
	averageSyncLen float64

	// syncEventDocCount is all sync events that we've ever processed for individual docs
	syncEventDocCount uint64

	// syncEventCollectionCount is all the sync events we've processed for collection watches
	syncEventCollectionCount uint64

	// requestCount is all requests that we've ever processed
	requestCount uint64

	// changeCount is the number of change events we've processed
	changeCount uint64

	// revnoMapBytes tracks how big our revnomap is in approximate bytes
	revnoMapBytes uintptr
}

// HubWatcherConfig contains the configuration parameters required
// for a NewHubWatcher.
type HubWatcherConfig struct {
	// Hub is the source of the events for the hub watcher.
	Hub HubSource
	// Clock allows tests to control the advancing of time.
	Clock Clock
	// ModelUUID refers to the model that this hub watcher is being
	// started for.
	ModelUUID string
	// Logger is used to control where the log messages for this watcher go.
	Logger Logger
}

// Validate ensures that all the values that have to be set are set.
func (config HubWatcherConfig) Validate() error {
	if config.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("missing Model UUID")
	}
	return nil
}

// NewHubWatcher returns a new watcher observing Change events published to the
// hub.
func NewHubWatcher(config HubWatcherConfig) (*HubWatcher, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "new HubWatcher invalid config")
	}
	watcher, _ := newHubWatcher(config.Hub, config.Clock, config.ModelUUID, config.Logger)
	return watcher, nil
}

func newHubWatcher(hub HubSource, clock Clock, modelUUID string, logger Logger) (*HubWatcher, <-chan struct{}) {
	if logger == nil {
		logger = noOpLogger{}
	}
	started := make(chan struct{})
	w := &HubWatcher{
		hub:       hub,
		clock:     clock,
		modelUUID: modelUUID,
		idleFunc:  HubWatcherIdleFunc,
		logger:    logger,
		watches:   make(map[watchKey][]watchInfo),
		current:   make(map[watchKey]int64),
		request:   make(chan interface{}),
		changes:   make(chan Change),
	}
	w.tomb.Go(func() error {
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
		return cause
	})
	return w, started
}

func (w *HubWatcher) receiveEvent(topic string, data interface{}) {
	switch topic {
	case txnWatcherStarting:
		// We don't do anything on a start.
	case txnWatcherSyncErr:
		w.tomb.Kill(errors.New("txn watcher sync error"))
	case txnWatcherCollection:
		change, ok := data.(Change)
		if !ok {
			w.logger.Warningf("incoming event not a Change")
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

// HubWatcherStats defines a few metrics that the hub watcher tracks
type HubWatcherStats struct {
	// WatchKeyCount is the number of keys being watched
	WatchKeyCount int
	// WatchCount is the number of watchers (keys can be watched by multiples)
	WatchCount uint64
	// RevnoMapSize is the number of keys stored in the 'current' map
	RevnoMapSize int
	// RevnoMapBytes is an approximation of the number of bytes being stored by 'current'
	RevnoMapBytes uint64
	// SyncQueueCap is the maximum buffer size for synchronization events
	SyncQueueCap int
	// SyncQueueLen is the current number of events being queued
	SyncQueueLen int
	// SyncLastLen was the length of SyncQueue the last time we flushed
	SyncLastLen int
	// SyncAvgLen is a smoothed average of recent sync lengths
	SyncAvgLen int
	// SyncMaxLen was the longest we've seen SyncQueue when flushing
	SyncMaxLen int
	// SyncEventDocCount is the number of sync events we've generated for specific documents
	SyncEventDocCount uint64
	// SyncEventCollCount is the number of sync events we've generated for documents changed in collections
	SyncEventCollCount uint64
	// RequestEventCount is the number of request events we've generated
	// (documents being watched that changed since the request came in)
	RequestEventCount uint64
	// RequestQueueCap is the maximum size of the request queue buffer
	RequestQueueCap int
	// RequestQueueLen is the current number of requested events
	RequestQueueLen int
	// RequestCount is the number of requests (reqWatch/reqUnwatch, etc) that we've seen
	RequestCount uint64
	// ChangeCount is the number of changes we've processed
	ChangeCount uint64
}

type reqStats struct {
	ch chan<- HubWatcherStats
}

func (w *HubWatcher) Stats() HubWatcherStats {
	ch := make(chan HubWatcherStats)
	w.sendReq(reqStats{ch: ch})
	select {
	case <-w.tomb.Dying():
		return HubWatcherStats{}
	case stats := <-ch:
		return stats
	}
}

// Report conforms to the worker.Runner.Report interface for returning information about the active worker.
func (w *HubWatcher) Report() map[string]interface{} {
	stats := w.Stats()
	return map[string]interface{}{
		"watch-count":           stats.WatchCount,
		"watch-key-count":       stats.WatchKeyCount,
		"revno-map-size":        stats.RevnoMapSize,
		"revno-map-bytes":       stats.RevnoMapBytes,
		"sync-queue-cap":        stats.SyncQueueCap,
		"sync-queue-len":        stats.SyncQueueLen,
		"sync-last-len":         stats.SyncLastLen,
		"sync-avg-len":          stats.SyncAvgLen,
		"sync-max-len":          stats.SyncMaxLen,
		"sync-event-doc-count":  stats.SyncEventDocCount,
		"sync-event-coll-count": stats.SyncEventCollCount,
		"request-queue-cap":     stats.RequestQueueCap,
		"request-queue-len":     stats.RequestQueueLen,
		"request-event-count":   stats.RequestEventCount,
		"request-count":         stats.RequestCount,
		"change-count":          stats.ChangeCount,
	}
}

// loop implements the main watcher loop.
// period is the delay between each sync.
func (w *HubWatcher) loop() error {
	w.logger.Tracef("loop started")
	defer w.logger.Tracef("loop finished")
	// idle is initially nil, and is only ever set if idleFunc
	// has a value.
	var idle <-chan time.Time
	if w.idleFunc != nil {
		w.logger.Tracef("set idle timeout to %s", HubWatcherIdleTime)
		idle = w.clock.After(HubWatcherIdleTime)
	}
	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case change := <-w.changes:
			w.queueChange(change)
		case req := <-w.request:
			w.handle(req)
		case <-idle:
			w.logger.Tracef("notify %s idle", w.modelUUID)
			w.idleFunc(w.modelUUID)
			idle = w.clock.After(HubWatcherIdleTime)
		}
		for (len(w.syncEvents) + len(w.requestEvents)) > 0 {
			select {
			case <-w.tomb.Dying():
				return errors.Trace(tomb.ErrDying)
			default:
				if w.flush() {
					if w.idleFunc != nil {
						w.logger.Tracef("set idle timeout to %s", HubWatcherIdleTime)
						idle = w.clock.After(HubWatcherIdleTime)
					}
				}
			}
		}
	}
}

func (w *HubWatcher) flush() bool {
	watchersNotified := false
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
				return watchersNotified
			case req := <-w.request:
				w.handle(req)
				continue
			case change := <-w.changes:
				w.queueChange(change)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
				w.logger.Tracef("e.ch=%v has been notified %v", e.ch, Change{e.key.c, e.key.id, e.revno})
				watchersNotified = true
			}
			break
		}
	}
	w.lastSyncLen = len(w.syncEvents)
	if w.lastSyncLen > w.maxSyncLen {
		w.maxSyncLen = w.lastSyncLen
	}
	// first-order filter: https://en.wikipedia.org/wiki/Low-pass_filter#Discrete-time_realization
	// This allows us to compute an "average" without having to actually track N samples.
	w.averageSyncLen = (filterFactor * float64(w.lastSyncLen)) + ((1.0 - filterFactor) * w.averageSyncLen)
	w.syncEvents = w.syncEvents[:0]
	// TODO(jam): 2018-11-07 This would probably be a good time to wipe syncEvents if cap(syncEvents) is significantly
	// larger than averageSyncLen. Consider something like "if cap(syncEventsLen) > 10*w.averageSyncLen".
	// That means that we can shrink the buffer after an outlier, rather than requiring it to always be the longest
	// it was ever needed.
	w.logger.Tracef("syncEvents: len(%d), cap(%d) avg(%.1f)", len(w.syncEvents), cap(w.syncEvents), w.averageSyncLen)

	// requestEvents are stored oldest first, and
	// may grow during the loop.
	for i := 0; i < len(w.requestEvents); i++ {
		// We need to reget the address value each time through the loop
		// as the slice may be reallocated.
		for e := &w.requestEvents[i]; e.ch != nil; e = &w.requestEvents[i] {
			select {
			case <-w.tomb.Dying():
				return watchersNotified
			case req := <-w.request:
				w.handle(req)
				continue
			case change := <-w.changes:
				w.queueChange(change)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
				w.logger.Tracef("e.ch=%v has been notified %v", e.ch, Change{e.key.c, e.key.id, e.revno})
				watchersNotified = true
			}
			break
		}
	}
	w.requestEvents = w.requestEvents[:0]
	return watchersNotified
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *HubWatcher) handle(req interface{}) {
	w.logger.Tracef("got request: %#v", req)
	w.requestCount++
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
	case reqStats:
		var watchCount uint64
		for _, watches := range w.watches {
			watchCount += uint64(len(watches))
		}
		stats := HubWatcherStats{
			ChangeCount:        w.changeCount,
			WatchKeyCount:      len(w.watches),
			WatchCount:         watchCount,
			RevnoMapSize:       len(w.current),
			RevnoMapBytes:      uint64(w.revnoMapBytes),
			SyncQueueCap:       cap(w.syncEvents),
			SyncQueueLen:       len(w.syncEvents),
			SyncLastLen:        w.lastSyncLen,
			SyncMaxLen:         w.maxSyncLen,
			SyncAvgLen:         int(w.averageSyncLen + 0.5),
			SyncEventCollCount: w.syncEventCollectionCount,
			SyncEventDocCount:  w.syncEventDocCount,
			RequestQueueCap:    cap(w.requestEvents),
			RequestQueueLen:    len(w.requestEvents),
			RequestCount:       w.requestCount,
		}
		select {
		case <-w.tomb.Dying():
			return
		case r.ch <- stats:
		}
	default:
		panic(fmt.Errorf("unknown request: %T", req))
	}
}

var (
	int64Size    = reflect.TypeOf(int64(0)).Size()
	strSize      = reflect.TypeOf("").Size()
	watchKeySize = reflect.TypeOf(watchKey{}).Size()
)

func sizeInMap(key watchKey) uintptr {
	// This includes the size of the int64 pointer that we target
	// TODO: (jam) 2018-11-05 there would be ways to be more accurate about the sizes,
	// but this is a rough approximation. It should handle the most common types that we know about,
	// where the id is a string
	size := int64Size
	size += watchKeySize
	size += strSize
	size += uintptr(len(key.c))
	switch id := key.id.(type) {
	case string:
		size += strSize
		size += uintptr(len(id))
	default:
		size += reflect.TypeOf(id).Size()
	}
	return size
}

// queueChange queues up the change for the registered watchers.
func (w *HubWatcher) queueChange(change Change) {
	w.changeCount++
	w.logger.Tracef("got change document: %#v", change)
	key := watchKey{change.C, change.Id}
	if _, ok := w.current[key]; !ok {
		// We didn't have this in our map before, so we need to track the bytes
		w.revnoMapBytes += sizeInMap(key)
	}
	revno := change.Revno
	w.current[key] = revno

	// Queue notifications for per-collection watches.
	for _, info := range w.watches[watchKey{change.C, nil}] {
		if info.filter != nil && !info.filter(change.Id) {
			continue
		}
		w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
		w.syncEventCollectionCount++
		w.logger.Tracef("adding collection watch for %v syncEvents: len(%d), cap(%d)", info.ch, len(w.syncEvents), cap(w.syncEvents))
	}

	// Queue notifications for per-document watches.
	infos := w.watches[key]
	for i, info := range infos {
		if revno > info.revno || revno < 0 && info.revno >= 0 {
			infos[i].revno = revno
			w.syncEvents = append(w.syncEvents, event{info.ch, key, revno})
			w.syncEventDocCount++
			w.logger.Tracef("adding document watch for %v syncEvents: len(%d), cap(%d)", info.ch, len(w.syncEvents), cap(w.syncEvents))
		}
	}
}
