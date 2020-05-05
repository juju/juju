// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
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

// syncEventSendTimeout is the amount of time we'll wait to send an
// event on a watch's channel before complaining about it.
const syncEventSendTimeout = 10 * time.Second

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

	// syncEvents contain the events to be
	// dispatched to the watcher channels. They're queued during
	// processing and flushed at the end to simplify the algorithm.
	syncEvents []event

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

// NewDead returns a new watcher that is already dead
// and always returns the given error from its Err method.
func NewDead(err error) *HubWatcher {
	w := &HubWatcher{
		logger: noOpLogger{},
	}
	w.tomb.Kill(errors.Trace(err))
	return w
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

func (w *HubWatcher) sendAndWaitReq(req waitableRequest) error {
	select {
	case w.request <- req:
	case <-w.tomb.Dying():
		return errors.Trace(tomb.ErrDying)
	}
	completed := req.Completed()
	select {
	case err := <-completed:
		return errors.Trace(err)
	case <-w.tomb.Dying():
		return errors.Trace(tomb.ErrDying)
	}
}

// WatchMulti watches a particular collection for several ids.
// The request is synchronous with the worker loop, so by the time the function returns,
// we guarantee that the watch is in place. If there is a mistake in the arguments (id is nil,
// channel is already watching a given id), an error will be returned and no watches will be added.
func (w *HubWatcher) WatchMulti(collection string, ids []interface{}, ch chan<- Change) error {
	for _, id := range ids {
		if id == nil {
			return errors.Errorf("cannot watch a document with nil id")
		}
	}
	req := reqWatchMulti{
		collection:  collection,
		ids:         ids,
		completedCh: make(chan error),
		watchCh:     ch,
		source:      debug.Stack(),
	}
	return errors.Trace(w.sendAndWaitReq(req))
}

// Watch starts watching the given collection and document id.
// An event will be sent onto ch whenever a matching document's txn-revno
// field is observed to change after a transaction is applied.
func (w *HubWatcher) Watch(collection string, id interface{}, ch chan<- Change) {
	if id == nil {
		panic("watcher: cannot watch a document with nil id")
	}
	// We use a value of -2 to indicate that we don't know the state of the document.
	// -1 would indicate that we think the document is deleted (and won't trigger
	// a change event if the document really is deleted).
	w.sendAndWaitReq(reqWatch{
		key: watchKey{collection, id},
		info: watchInfo{
			ch:     ch,
			revno:  -2,
			filter: nil,
			source: debug.Stack(),
		},
		registeredCh: make(chan error),
	})
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
	w.sendAndWaitReq(reqWatch{
		key: watchKey{collection, nil},
		info: watchInfo{
			ch:     ch,
			revno:  0,
			filter: filter,
			source: debug.Stack(),
		},
		registeredCh: make(chan error),
	})
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
	// RequestCount is the number of requests (reqWatch/reqUnwatch, etc) that we've seen
	RequestCount uint64
	// ChangeCount is the number of changes we've processed
	ChangeCount uint64
}

type reqStats struct {
	ch chan<- HubWatcherStats
}

func (r reqStats) String() string {
	return fmt.Sprintf("%#v", r)
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
		"sync-queue-cap":        stats.SyncQueueCap,
		"sync-queue-len":        stats.SyncQueueLen,
		"sync-last-len":         stats.SyncLastLen,
		"sync-avg-len":          stats.SyncAvgLen,
		"sync-max-len":          stats.SyncMaxLen,
		"sync-event-doc-count":  stats.SyncEventDocCount,
		"sync-event-coll-count": stats.SyncEventCollCount,
		"request-count":         stats.RequestCount,
		"change-count":          stats.ChangeCount,
	}
}

// loop implements the main watcher loop.
// period is the delay between each sync.
func (w *HubWatcher) loop() error {
	w.logger.Tracef("%p loop started", w)
	defer w.logger.Tracef("loop finished")
	// idle is initially nil, and is only ever set if idleFunc
	// has a value.
	var idle <-chan time.Time
	if w.idleFunc != nil {
		w.logger.Tracef("%p set idle timeout to %s", w, HubWatcherIdleTime)
		idle = time.After(HubWatcherIdleTime)
	}
	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case inChange := <-w.changes:
			w.queueChange(inChange)
			if w.idleFunc != nil {
				idle = time.After(HubWatcherIdleTime)
			}
		case req := <-w.request:
			w.handle(req)
		case <-idle:
			w.logger.Tracef("%p notify %s idle", w, w.modelUUID)
			w.idleFunc(w.modelUUID)
			idle = time.After(HubWatcherIdleTime)
		}
		for len(w.syncEvents) > 0 {
			select {
			case <-w.tomb.Dying():
				return errors.Trace(tomb.ErrDying)
			default:
				if w.flush() {
					if w.idleFunc != nil {
						idle = time.After(HubWatcherIdleTime)
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
	w.logger.Tracef("%p flushing syncEvents: len(%d) cap(%d)", w, len(w.syncEvents), cap(w.syncEvents))
allevents:
	for i := 0; i < len(w.syncEvents); i++ {
		deadline := w.clock.After(syncEventSendTimeout)
		// We need to reget the address value each time through the loop
		// as the slice may be reallocated.
		for e := &w.syncEvents[i]; e.ch != nil; e = &w.syncEvents[i] {
			outChange := Change{
				C:     e.key.c,
				Id:    e.key.id,
				Revno: e.revno,
			}
			w.logger.Tracef("%p sending syncEvent(%d): e.ch=%v %v", w, i, e.ch, outChange)
			select {
			case <-w.tomb.Dying():
				return watchersNotified
			case req := <-w.request:
				w.handle(req)
				continue
			case inChange := <-w.changes:
				w.queueChange(inChange)
				continue
			case e.ch <- outChange:
				w.logger.Tracef("%p e.ch=%v has been notified %v", w, e.ch, outChange)
				watchersNotified = true
			case <-deadline:
				w.logger.Criticalf("%p programming error, e.ch=%v did not accept %v - missing Unwatch?\nwatch source:\n%s",
					w, e.ch, outChange, e.watchSource)
				continue allevents
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
	w.logger.Tracef("%p syncEvents after flush: len(%d), cap(%d) avg(%.1f)", w, len(w.syncEvents), cap(w.syncEvents), w.averageSyncLen)
	if cap(w.syncEvents) > 100 && float64(cap(w.syncEvents)) > 10.0*w.averageSyncLen {
		w.logger.Debugf("syncEvents buffer being reset from peak size %d", cap(w.syncEvents))
		w.syncEvents = nil
	}

	return watchersNotified
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *HubWatcher) handle(req interface{}) {
	w.logger.Tracef("%p got request: %s", w, req)
	w.requestCount++
	switch r := req.(type) {
	case reqWatch:
		// TODO(jam): 2019-01-30 Now that reqWatch has a registeredCh, we could have Watch return an error
		// rather than panic()
		for _, info := range w.watches[r.key] {
			if info.ch == r.info.ch {
				panic(fmt.Errorf("tried to re-add channel %v for %s", info.ch, r.key))
			}
		}
		w.watches[r.key] = append(w.watches[r.key], r.info)
		if r.registeredCh != nil {
			select {
			case r.registeredCh <- nil:
			case <-w.tomb.Dying():
			}
		}
	case reqWatchMulti:
		for _, id := range r.ids {
			key := watchKey{c: r.collection, id: id}
			for _, info := range w.watches[key] {
				if info.ch == r.watchCh {
					err := errors.Errorf("tried to re-add channel %v for %s", r.watchCh, key)
					select {
					case r.completedCh <- err:
					case <-w.tomb.Dying():
					}
					return
				}
			}
		}
		for _, id := range r.ids {
			key := watchKey{c: r.collection, id: id}
			info := watchInfo{
				ch:     r.watchCh,
				revno:  -2,
				filter: nil,
				source: r.source,
			}
			w.watches[key] = append(w.watches[key], info)
		}
		select {
		case r.completedCh <- nil:
		case <-w.tomb.Dying():
		}
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
			SyncQueueCap:       cap(w.syncEvents),
			SyncQueueLen:       len(w.syncEvents),
			SyncLastLen:        w.lastSyncLen,
			SyncMaxLen:         w.maxSyncLen,
			SyncAvgLen:         int(w.averageSyncLen + 0.5),
			SyncEventCollCount: w.syncEventCollectionCount,
			SyncEventDocCount:  w.syncEventDocCount,
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
	w.logger.Tracef("%p got change document: %#v", w, change)
	key := watchKey{change.C, change.Id}
	revno := change.Revno

	// Queue notifications for per-collection watches.
	for _, info := range w.watches[watchKey{change.C, nil}] {
		if info.filter != nil && !info.filter(change.Id) {
			continue
		}
		evt := event{
			ch:          info.ch,
			key:         key,
			revno:       revno,
			watchSource: info.source,
		}
		w.syncEvents = append(w.syncEvents, evt)
		w.syncEventCollectionCount++
		w.logger.Tracef("%p adding event for collection %q watch %v, syncEvents: len(%d), cap(%d)", w, change.C, info.ch, len(w.syncEvents), cap(w.syncEvents))
	}

	// Queue notifications for per-document watches.
	infos := w.watches[key]
	for i, info := range infos {
		if revno > info.revno || revno < 0 && info.revno >= 0 {
			infos[i].revno = revno
			evt := event{
				ch:          info.ch,
				key:         key,
				revno:       revno,
				watchSource: info.source,
			}
			w.syncEvents = append(w.syncEvents, evt)
			w.syncEventDocCount++
			w.logger.Tracef("%p adding event for %v watch %v, syncEvents: len(%d), cap(%d)", w, key, info.ch, len(w.syncEvents), cap(w.syncEvents))
		}
	}
}
