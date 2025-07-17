// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
)

var (
	// HubWatcherIdleFunc allows tests to be able to get callbacks
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

// HubWatcher listens to events from the hub and passes them on to the registered
// watchers.
type HubWatcher struct {
	tomb tomb.Tomb
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
	Logger logger.Logger
}

// Validate ensures that all the values that have to be set are set.
func (config HubWatcherConfig) Validate() error {
	return nil
}

// NewHubWatcher returns a new watcher observing Change events published to the
// hub.
func NewHubWatcher(config HubWatcherConfig) (*HubWatcher, error) {
	watcher, _ := newHubWatcher()
	return watcher, nil
}

// NewDead returns a new watcher that is already dead
// and always returns the given error from its Err method.
func NewDead(err error) *HubWatcher {
	w := &HubWatcher{}
	w.tomb.Kill(errors.Trace(err))
	return w
}

func newHubWatcher() (*HubWatcher, <-chan struct{}) {
	started := make(chan struct{})
	w := &HubWatcher{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w, started
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

// WatchMulti watches a particular collection for several ids.
// The request is synchronous with the worker loop, so by the time the function returns,
// we guarantee that the watch is in place. If there is a mistake in the arguments (id is nil,
// channel is already watching a given id), an error will be returned and no watches will be added.
func (w *HubWatcher) WatchMulti(collection string, ids []interface{}, ch chan<- Change) error {
	return nil
}

// Watch starts watching the given collection and document id.
// An event will be sent onto ch whenever a matching document's txn-revno
// field is observed to change after a transaction is applied.
func (w *HubWatcher) Watch(collection string, id interface{}, ch chan<- Change) {
}

// WatchCollection starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection.
func (w *HubWatcher) WatchCollection(collection string, ch chan<- Change) {
}

// WatchCollectionWithFilter starts watching the given collection.
// An event will be sent onto ch whenever the txn-revno field is observed
// to change after a transaction is applied for any document in the collection, so long as the
// specified filter function returns true when called with the document id value.
func (w *HubWatcher) WatchCollectionWithFilter(collection string, ch chan<- Change, filter func(interface{}) bool) {
}

// Unwatch stops watching the given collection and document id via ch.
func (w *HubWatcher) Unwatch(collection string, id interface{}, ch chan<- Change) {
}

// UnwatchCollection stops watching the given collection via ch.
func (w *HubWatcher) UnwatchCollection(collection string, ch chan<- Change) {
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

func (w *HubWatcher) Stats() HubWatcherStats {
	return HubWatcherStats{}
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
