// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
)

// The cached controller includes a "residentManager", which supplies new
// cache "Resident" instances, monitors their life cycles and is the source
// of unique identifiers for residents and resources.
// All cached entities aside from the parent controller should include
// the "resident" type as a base.
// Each resident monitors resources that it creates and is responsible for
// cleaning up when it is to be evicted from the cache.
// This design meets resource management requirements multi-directionally:
//   1. Resources (such as watchers) that are stopped or otherwise destroyed
//      by their upstream owners need to be deregistered from their residents.
//   2. Residents removed from a model in the normal course of events need to
//      release resources that they created and deregister from the controller.
//   3. If the multi-watcher supplying deltas to the cache is restarted,
//      The controller itself must mark and sweep, evicting stale residents and
//      cleaning up their resources.
// Where possible the manager and residents eschew responsibility for Goroutine
// safety. The types into which they are embedded should handle this.

// counter supplies monotonically increasing unique identifiers.
type counter uint64

// next returns the next identifier, incrementing the counter.
func (c *counter) next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}

// last returns the current value of the counter.
// Used for testing and diagnostics.
func (c *counter) last() uint64 {
	return atomic.LoadUint64((*uint64)(c))
}

// residentManager creates and tracks cache residents.
// It is also the source for resource identifiers in the cache.
type residentManager struct {
	residentCount *counter
	resourceCount *counter

	residents map[uint64]*Resident
}

func newResidentManager() *residentManager {
	residentC := counter(0)
	resourceC := counter(0)

	return &residentManager{
		residentCount: &residentC,
		resourceCount: &resourceC,
		residents:     make(map[uint64]*Resident),
	}
}

// new creates a uniquely identified type-agnostic cache resident,
// registers it in the internal map, then returns it.
func (m *residentManager) new() *Resident {
	id := m.residentCount.next()

	r := &Resident{
		id:             id,
		deregister:     func() { m.deregister(id) },
		nextResourceId: func() uint64 { return m.resourceCount.next() },
		workers:        make(map[uint64]worker.Worker),
	}
	m.residents[r.id] = r
	return r
}

func (m *residentManager) evict(id uint64) {
	// TODO (manadart 2019-04-17): TBC when the mark/sweep logic is added.
}

func (m *residentManager) deregister(id uint64) {
	delete(m.residents, id)
}

// Resident is the base class for entities managed in the cache.
type Resident struct {
	// id uniquely identifies this resident among all
	// that were supplied by the same resident manager.
	id uint64

	// stale indicates that this cache resident is stale
	// and is a candidate for removal.
	stale bool

	// deregister removes this resident from the manager that instantiated it.
	deregister func()

	// nextResourceId is a factory method for acquiring unique resource IDs.
	nextResourceId func() uint64

	// workers are resources that must be cleaned up when a resident is to be
	// evicted from the cache.
	// Obvious examples are watchers created by the resident.
	// Access to this map should be Goroutine-safe.
	workers map[uint64]worker.Worker
	mu      sync.Mutex
}

// CacheId returns the unique ID for this cache resident.
func (r *Resident) CacheId() uint64 {
	return r.id
}

// registerWorker is used to indicate that the input worker needs to be stopped
// when this resident is evicted from the cache.
// The deregistration method is returned.
// TODO (manadart 2019-04-16): Handle case where registration is called
// on a stale resident.
func (r *Resident) registerWorker(w worker.Worker) func() {
	id := r.nextResourceId()
	r.mu.Lock()
	r.workers[id] = w
	r.mu.Unlock()
	return func() { r.deregisterWorker(id) }
}

// evict cleans up any resources created by this resident,
// then deregisters it.
func (r *Resident) evict() error {
	if err := r.cleanup(); err != nil {
		return errors.Trace(err)
	}
	r.deregister()
	return nil
}

// cleanup performs all resource maintenance associated with a resident
// being evicted from the cache.
// Note that this method does not deregister the resident from the manager.
func (r *Resident) cleanup() error {
	return errors.Annotatef(r.cleanupWorkers(), "cleaning up cache resident %d:", r.id)
}

// cleanupWorkers calls "Stop" on all registered workers
// and removes them from the internal map.
func (r *Resident) cleanupWorkers() error {
	var errs []string
	for id := range r.workers {
		if err := r.cleanupWorker(id); err != nil {
			errs = append(errs, errors.Annotatef(err, "worker %d", id).Error())
		}
	}

	if len(errs) != 0 {
		return errors.Errorf("worker cleanup errors:\n\t%s", strings.Join(errs, "\n\t"))
	}
	return nil
}

// cleanupWorker stops and deregisters the worker with the input ID.
// If no such worker is found, an error is returned.
// Note that the deregistration method should have been added the the worker's
// tomb cleanup method - stopping the worker cleanly is enough to deregister.
func (r *Resident) cleanupWorker(id uint64) error {
	w, ok := r.workers[id]
	if !ok {
		return errors.Errorf("worker %d not found", id)
	}

	if err := worker.Stop(w); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// deregisterWorker informs the resident that we no longer care about this
// worker. We expect this call to come from workers stopped by other actors
// other than the resident, so we ensure Goroutine safety.
func (r *Resident) deregisterWorker(id uint64) {
	r.mu.Lock()
	delete(r.workers, id)
	r.mu.Unlock()
}
