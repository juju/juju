// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/errors"
)

// entityFreshness represents the current entity lifecycle.
type entityFreshness uint8

const (
	// stale represents a lifecycle state that defines if an entity is stale
	// and isn't currently active.
	stale entityFreshness = iota
	// Active lifecycle state defines a entity state that represents a the
	// entity is currently active.
	fresh
)

// entity represents a base entity within the model cache
type entity struct {
	freshness    entityFreshness
	mu           sync.Mutex
	removalDelta func() interface{}
	watchers     []Watcher
}

// mark updates the state to be classified as stale.
func (e *entity) mark() {
	e.mu.Lock()
	e.freshness = stale
	e.mu.Unlock()
}

// sweep goes through and creates a set of deltas for the entity. That way
// we can feed that back into the controller so it cleans up the entities in
// one code path.
func (e *entity) sweep() (*SweepDeltas, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.freshness == stale {
		var deltas []interface{}
		if e.removalDelta == nil {
			// If no removalDelta is found, this a programmatic error, but we
			// should ensure that we surface that error accordingly.
			return nil, errors.New("removalDelta is required when performing a sweep")
		}
		if delta := e.removalDelta(); delta != nil {
			deltas = append(deltas, delta)
		}

		return &SweepDeltas{
			Deltas: deltas,
		}, nil
	}
	return &SweepDeltas{
		FreshCount: 1,
	}, nil
}

// SweepInfo represents the information gathered whilst doing a sweep
type SweepInfo struct {
	StaleCount int
	FreshCount int
}

// SweepDeltas represents what deltas are required when walking over all
// entities in a cache to see which are active and stale.
type SweepDeltas struct {
	Deltas     []interface{}
	FreshCount int
}

// Merge concatenates two SweepDeltas together
func (d *SweepDeltas) Merge(o *SweepDeltas) {
	d.Deltas = append(d.Deltas, o.Deltas...)
	d.FreshCount += o.FreshCount
}
