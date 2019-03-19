// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"sync"

	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"
)

// The watchers used in the cache package are closer to state watchers
// than core watchers. The core watchers never close their changes channel,
// which leads to issues in the apiserver facade methods dealing with
// watchers. So the watchers in this package do close their changes channels.

// Watcher is the common methods
type Watcher interface {
	worker.Worker
	// Stop is currently needed by the apiserver until the resources
	// work on workers instead of things that can be stopped.
	Stop() error
}

// NotifyWatcher will only say something changed.
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}

type notifyWatcherBase struct {
	tomb    tomb.Tomb
	changes chan struct{}
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex
}

func newNotifyWatcherBase() *notifyWatcherBase {
	// We use a single entry buffered channel for the changes.
	// This allows the config changed handler to send a value when there
	// is a change, but if that value hasn't been consumed before the
	// next change, the second change is discarded.
	ch := make(chan struct{}, 1)

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	ch <- struct{}{}

	return &notifyWatcherBase{changes: ch}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *notifyWatcherBase) Changes() <-chan struct{} {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *notifyWatcherBase) Kill() {
	w.mu.Lock()
	w.closed = true
	close(w.changes)
	w.mu.Unlock()
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *notifyWatcherBase) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *notifyWatcherBase) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *notifyWatcherBase) notify() {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()
		return
	}

	select {
	case w.changes <- struct{}{}:
	default:
		// Already a pending change, so do nothing.
	}

	w.mu.Unlock()
}

// ConfigWatcher watches a single entity's configuration.
// If keys are specified the watcher only signals a change when at least one
// of those keys changes value. If no keys are specified,
// any change in the config will trigger the watcher to notify.
type ConfigWatcher struct {
	*notifyWatcherBase

	keys []string
	hash string
}

// newConfigWatcher returns a new watcher for the input config keys
// with a baseline hash of their config values from the input hash cache.
// As per the cache requirements, hashes are only generated from sorted keys.
func newConfigWatcher(keys []string, cache *hashCache, hub *pubsub.SimpleHub, topic string) *ConfigWatcher {
	sort.Strings(keys)

	w := &ConfigWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		keys: keys,
		hash: cache.getHash(keys),
	}

	unsub := hub.Subscribe(topic, w.configChanged)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	return w
}

func (w *ConfigWatcher) configChanged(topic string, value interface{}) {
	hashCache, ok := value.(*hashCache)
	if !ok {
		logger.Errorf("programming error, value not of type *hashCache")
	}
	hash := hashCache.getHash(w.keys)
	if hash == w.hash {
		// Nothing that we care about has changed, so we're done.
		return
	}
	w.notify()
}

// StringsWatcher will return what has changed.
type StringsWatcher interface {
	Watcher
	Changes() <-chan []string
}

type stringsWatcherBase struct {
	tomb    tomb.Tomb
	changes chan []string
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex
}

func newStringsWatcherBase(values ...string) *stringsWatcherBase {
	// We use a single entry buffered channel for the changes.
	// This allows the config changed handler to send a value when there
	// is a change, but if that value hasn't been consumed before the
	// next change, the second change is discarded.
	ch := make(chan []string, 1)

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	ch <- values

	return &stringsWatcherBase{changes: ch}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *stringsWatcherBase) Changes() <-chan []string {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *stringsWatcherBase) Kill() {
	w.mu.Lock()
	w.closed = true
	close(w.changes)
	w.mu.Unlock()
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stringsWatcherBase) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *stringsWatcherBase) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *stringsWatcherBase) notify(values []string) {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()
		return
	}

	select {
	case w.changes <- values:
	default:
		// Already a pending change, so do nothing.
	}

	w.mu.Unlock()
}

// ChangeWatcher notifies that something changed, with
// the given slice of strings.  An initial event is sent
// with the input given at creation.
type ChangeWatcher struct {
	*stringsWatcherBase
}

func newAddRemoveWatcher(values ...string) *ChangeWatcher {
	return &ChangeWatcher{
		stringsWatcherBase: newStringsWatcherBase(values...),
	}
}

func (w *ChangeWatcher) changed(topic string, value interface{}) {
	strings, ok := value.([]string)
	if !ok {
		logger.Errorf("programming error, value not of type []string")
	}

	w.changes <- strings
}
