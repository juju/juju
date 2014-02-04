// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package voyeur implements a concurrency-safe value that can be watched for
// changes.
package voyeur

import (
	"sync"
)

// Value represents a shared value that can be watched for changes. Methods on
// a Value may be called concurrently.
type Value struct {
	val     interface{}
	version int
	mu      sync.RWMutex
	wait    sync.Cond
	closed  bool
}

// NewValue creates a new Value holding the given initial value. If initial is
// nil, any watchers will wait until a value is set.
func NewValue(initial interface{}) *Value {
	v := new(Value)
	v.wait.L = v.mu.RLocker()
	if initial != nil {
		v.val = initial
		v.version++
	}
	return v
}

// Set sets the shared value to val.
func (v *Value) Set(val interface{}) {
	v.mu.Lock()
	v.val = val
	v.version++
	v.wait.Broadcast()
	v.mu.Unlock()
}

// Close closes the Value, unblocking any outstanding watchers.  Close always
// returns nil.
func (v *Value) Close() error {
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()
	v.wait.Broadcast()
	return nil
}

// Get returns the current value. If the Value has been closed, ok will be
// false.
func (v *Value) Get() (val interface{}, ok bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.closed {
		return nil, false
	}
	return v.val, true
}

// Watcher returns a Watcher that can be used to watch for changes to the value.
func (v *Value) Watcher() *Watcher {
	return &Watcher{value: v}
}

// Watcher represents a single watcher of a shared value.
type Watcher struct {
	value   *Value
	version int
	current interface{}
}

// Next blocks until there is a new value to be retrieved from the value that is
// being watched. It also unblocks when the value is closed. Next returns
// whether the value has been closed.
func (w *Watcher) Next() bool {
	w.value.mu.RLock()
	defer w.value.mu.RUnlock()

	// We should never go around this loop more than twice.
	for {
		if w.version != w.value.version {
			w.version = w.value.version
			w.current = w.value.val
			return true
		}
		if w.value.closed {
			return false
		}

		// wait is magic sauce that releases the lock until triggered
		// and then reacquires the lock, thus avoiding a deadlock.
		w.value.wait.Wait()
	}
}

// Value returns the last value that was retrieved from the watched Value.
func (w *Watcher) Value() interface{} {
	return w.current
}
