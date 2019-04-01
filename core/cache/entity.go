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
	state State
	mu    sync.Mutex
}

func (e *Entity) mark() {
	e.mu.Lock()
	e.state = Stale
	e.mu.Unlock()
}

func (e *Entity) isStale() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state == Stale
}

// staler represents a way to call isStale on any entity model item, without
// casting between the item and the base entity item.
type staler interface {
	isStale() bool
}

// SweepChecker represents a checker to walk over all entities in a cache
// to see which are active and stale.
type SweepChecker struct {
	Active, Stale int
}

// Check takes an Entity and works out if an entity is stale or active.
// Returns true if additional work when it's stale is required.
func (s *SweepChecker) Check(e staler) bool {
	res := e.isStale()
	if res {
		s.Stale++
	} else {
		s.Active++
	}
	return res
}
