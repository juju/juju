// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import "sync"

// State represents the current entity lifecycle.
type State uint8

const (
	// Stale represents a lifecycle state that defines if an entity is stale
	// and isn't currently active.
	Stale State = iota
	// Active lifecycle state defines a entity state that represents a the
	// entity is currently active.
	Active
)

// Entity represents a base entity within the model cache
type Entity struct {
	state    State
	mu       sync.Mutex
	watchers []Watcher
}

// mark updates the state to be classified as stale.
func (e *Entity) mark() {
	e.mu.Lock()
	e.state = Stale
	e.mu.Unlock()
}

// sweep goes through and creates a set of deltas for the entity. That way
// we can feed that back into the controller so it cleans up the entities in
// one code path.
func (e *Entity) sweep() *SweepDeltas {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == Stale {
		var deltas []interface{}
		if delta := e.RemovalDelta(); delta != nil {
			deltas = append(deltas, delta)
		}

		return &SweepDeltas{
			Deltas: deltas,
		}
	}
	return &SweepDeltas{
		Active: 1,
	}
}

// registerWatcher allows the tracking of a watcher, so when removing we can
// kill the watcher.
func (e *Entity) registerWatcher(w Watcher) {
	e.mu.Lock()
	e.watchers = append(e.watchers, w)
	e.mu.Unlock()
}

// remove is called when the entity is being cleaned up, so it can kill the
// watchers.
func (e *Entity) remove() {
	for _, watcher := range e.watchers {
		watcher.Kill()
	}
}

// RemovalDelta is called when the entity is marked as stale. The return value
// is then the RemoveEntity struct for the entity.
func (e *Entity) RemovalDelta() interface{} {
	return nil
}

// SweepInfo represents the information gathered whilst doing a sweep
type SweepInfo struct {
	Stale  int
	Active int
}

// SweepDeltas represents what deltas are required when walking over all
// entities in a cache to see which are active and stale.
type SweepDeltas struct {
	Deltas []interface{}
	Active int
}

func (d *SweepDeltas) Merge(o *SweepDeltas) {
	d.Deltas = append(d.Deltas, o.Deltas)
	d.Active += o.Active
}
