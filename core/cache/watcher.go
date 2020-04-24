// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"regexp"
	"sort"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
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
func (w *notifyWatcherBase) Changes() <-chan struct{} {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *notifyWatcherBase) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// The watcher must be dying or dead before we close the channel.
	// Otherwise readers could fail, but the watcher's tomb would indicate
	// "still alive".
	w.tomb.Kill(nil)
	w.closed = true
	close(w.changes)
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
func newConfigWatcher(
	keys []string, cache *hashCache, hub *pubsub.SimpleHub, topic string, res *Resident,
) *ConfigWatcher {
	sort.Strings(keys)

	w := &ConfigWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		keys: keys,
		hash: cache.getHash(keys),
	}

	deregister := res.registerWorker(w)
	unsub := hub.Subscribe(topic, w.configChanged)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		deregister()
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
	// is a change, if that value hasn't been consumed before the
	// next change, the changes are combined.
	ch := make(chan []string, 1)

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	ch <- values

	return &stringsWatcherBase{changes: ch}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *stringsWatcherBase) Changes() <-chan []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *stringsWatcherBase) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// The watcher must be dying or dead before we close the channel.
	// Otherwise readers could fail, but the watcher's tomb would indicate
	// "still alive".
	w.tomb.Kill(nil)
	w.closed = true
	close(w.changes)
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

// Err returns the inner tomb's error.
func (w *stringsWatcherBase) Err() error {
	return w.tomb.Err()
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
		// Already a pending change, so add the new values to the
		// pending change.
		w.amendBufferedChange(values)
	}

	w.mu.Unlock()
}

// amendBufferedChange alters the buffered notification to include new
// information. This method assumes lock protection.
func (w *stringsWatcherBase) amendBufferedChange(values []string) {
	select {
	case old := <-w.changes:
		w.changes <- set.NewStrings(old...).Union(set.NewStrings(values...)).Values()
	default:
		// Someone read the channel in the meantime.
		// We know we're locked, so no further writes will have occurred.
		// Just send what we were going to send.
		w.changes <- values
	}
}

// PredicateStringsWatcher notifies that something changed, with
// a slice of strings. The predicateFunc will test the values
// before notification. An initial event is sent with the input
// given at creation.
type PredicateStringsWatcher struct {
	*stringsWatcherBase

	fn predicateFunc
}

// newChangeWatcher provides a PredicateStringsWatcher which notifies
// with all strings passed to it.
func newChangeWatcher(values ...string) *PredicateStringsWatcher {
	return &PredicateStringsWatcher{
		stringsWatcherBase: newStringsWatcherBase(values...),
		fn:                 func(string) bool { return true },
	}
}

func regexpPredicate(compiled *regexp.Regexp) func(string) bool {
	return func(value string) bool { return compiled.MatchString(value) }
}

type predicateFunc func(string) bool

// newPredicateStringsWatcher provides a PredicateStringsWatcher which notifies
// with any string which passes the predicateFunc.
func newPredicateStringsWatcher(fn predicateFunc, values ...string) *PredicateStringsWatcher {
	return &PredicateStringsWatcher{
		stringsWatcherBase: newStringsWatcherBase(values...),
		fn:                 fn,
	}
}

func (w *PredicateStringsWatcher) changed(topic string, value interface{}) {
	strings, ok := value.([]string)
	if !ok {
		logger.Errorf("programming error, value not of type []string")
	}

	matches := set.NewStrings()
	for _, s := range strings {
		if w.fn(s) {
			matches.Add(s)
		}
	}

	if !matches.IsEmpty() {
		w.notify(matches.Values())
	}
}
