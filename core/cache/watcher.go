// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"sync"

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
// If keys are specified the watcher is only signals a change when at lest one
// of those keys change values. If no keys are specified,
// any change in the config will trigger the watcher to notify.
type ConfigWatcher struct {
	*notifyWatcherBase

	keys []string
	hash string
}

func newConfigWatcher(keys []string, keyHash string) *ConfigWatcher {
	sort.Strings(keys)

	return &ConfigWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		keys: keys,
		hash: keyHash,
	}
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
