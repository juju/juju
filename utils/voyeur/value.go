// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package voyeur implements a concurrency-safe value that can be watched for
// changes.
package voyeur

import (
	"sync"
)

// Value represents a shared value that can be watched for changes. Methods on
// a Value may be called concurrently. The zero Value is
// ok to use, and is equivalent to a NewValue result
// with a nil initial value.
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
	v.init()
	if initial != nil {
		v.val = initial
		v.version++
	}
	return v
}

func (v *Value) needsInit() bool {
	return v.wait.L == nil
}

func (v *Value) init() {
	if v.needsInit() {
		v.wait.L = v.mu.RLocker()
	}
}

// Set sets the shared value to val.
func (v *Value) Set(val interface{}) {
	v.mu.Lock()
	v.init()
	v.val = val
	v.version++
	v.mu.Unlock()
	v.wait.Broadcast()
}

// Close closes the Value, unblocking any outstanding watchers.  Close always
// returns nil.
func (v *Value) Close() error {
	v.mu.Lock()
	v.init()
	v.closed = true
	v.mu.Unlock()
	v.wait.Broadcast()
	return nil
}

// Closed reports whether the value has been closed.
func (v *Value) Closed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.closed
}

// Get returns the current value.
func (v *Value) Get() interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.val
}

// Watch returns a Watcher that can be used to watch for changes to the value.
func (v *Value) Watch() *Watcher {
	return &Watcher{value: v}
}

// Watcher represents a single watcher of a shared value.
type Watcher struct {
	value   *Value
	version int
	current interface{}
	closed  bool
}

// Next blocks until there is a new value to be retrieved from the value that is
// being watched. It also unblocks when the value or the Watcher itself is
// closed. Next returns false if the value or the Watcher itself have been
// closed.
func (w *Watcher) Next() bool {
	val := w.value
	val.mu.RLock()
	defer val.mu.RUnlock()
	if val.needsInit() {
		val.mu.RUnlock()
		val.mu.Lock()
		val.init()
		val.mu.Unlock()
		val.mu.RLock()
	}

	// We can go around this loop a maximum of two times,
	// because the only thing that can cause a Wait to
	// return is for the condition to be triggered,
	// which can only happen if the value is set (causing
	// the version to increment) or it is closed
	// causing the closed flag to be set.
	// Both these cases will cause Next to return.
	for {
		if w.version != val.version {
			w.version = val.version
			w.current = val.val
			return true
		}
		if val.closed || w.closed {
			return false
		}

		// Wait releases the lock until triggered and then reacquires the lock,
		// thus avoiding a deadlock.
		val.wait.Wait()
	}
}

// Close closes the Watcher without closing the underlying
// value. It may be called concurrently with Next.
func (w *Watcher) Close() {
	w.value.mu.Lock()
	w.value.init()
	w.closed = true
	w.value.mu.Unlock()
	w.value.wait.Broadcast()
}

// Value returns the last value that was retrieved from the watched Value by
// Next.
func (w *Watcher) Value() interface{} {
	return w.current
}
