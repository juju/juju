// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
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

	// residents are all the residents of the cache indexed by ID.
	// Access to this map should be protected by the Mutex below.
	residents map[uint64]*Resident
	mu        sync.Mutex

	// marked indicates whether a cache mark has been run,
	// but not yet any corresponding sweep.
	marked bool

	// removals is the channel on which remove messages are sent.
	// It will generally be the the cached controller's "changes" channel.
	removals chan<- interface{}

	// dying tells us that the manager's owner is going away.
	// This will generally correspond with the cached controller's
	// tomb.Dying channel.
	dying <-chan struct{}
}

func newResidentManager(removals chan<- interface{}) *residentManager {
	residentC := counter(0)
	resourceC := counter(0)

	return &residentManager{
		residentCount: &residentC,
		resourceCount: &resourceC,
		residents:     make(map[uint64]*Resident),
		removals:      removals,
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

	m.mu.Lock()
	m.residents[r.id] = r
	m.mu.Unlock()

	return r
}

// mark sets all of the manager's residents to be stale.
// If there were residents to mark, we set "marked" to be true.
func (m *residentManager) mark() {
	m.mu.Lock()
	for _, r := range m.residents {
		r.setStale(true)
	}
	m.marked = len(m.residents) > 0
	m.mu.Unlock()
}

// sweep removes stale cache residents in descending order of ID.
// Lock protection of the resident map is done in "evictions" and in the
// the ultimate "evict" invocations.
func (m *residentManager) sweep() <-chan struct{} {
	finished := make(chan struct{})

	// If we are not marked, there is no work to do.
	if !m.isMarked() {
		close(finished)
		return finished
	}

	removalIds, removalMessages := m.evictions()

	go func() {
		defer close(finished)
		for _, id := range removalIds {
			select {
			case m.removals <- removalMessages[id]:
			case <-m.dying:
				logger.Debugf("aborting cache sweep")
				return
			}
		}
	}()

	m.setMarked(false)
	return finished
}

// evictions iterates over the cache residents and generates a map of
// eviction messages and a slice for determining order of eviction.
// Because the IDs are supplied in increasing order,
// this ensures we never remove a resident's model before the resident itself.
func (m *residentManager) evictions() ([]uint64, map[uint64]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a descending order slice of stale IDs,
	// and a map of resident removal messages.
	var removalIds []uint64
	removalMessages := make(map[uint64]interface{})
	for id, r := range m.residents {
		if r.isStale() {
			if r.removalMessage == nil {
				logger.Warningf("cache resident %d has no removal message; skipping eviction", id)
				continue
			}

			removalIds = append(removalIds, id)
			removalMessages[id] = r.removalMessage
		}
	}
	sort.Sort(uint64Reverse(removalIds))

	return removalIds, removalMessages
}

func (m *residentManager) deregister(id uint64) {
	m.mu.Lock()
	delete(m.residents, id)
	m.mu.Unlock()
}

func (m *residentManager) isMarked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.marked
}

func (m *residentManager) setMarked(marked bool) {
	m.mu.Lock()
	m.marked = marked
	m.mu.Unlock()
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

	// removalMessage is a message that will be recognised by the cached
	// controller, for removing the resident's specific type from the cache.
	// See changes.go for the types of messages.
	removalMessage interface{}

	// workers are resources that must be cleaned up when a resident is to be
	// evicted from the cache.
	// Obvious examples are watchers created by the resident.
	// Access to this map should be protected with the Mutex below.
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

// cleanupWorkers calls "Stop" on all registered workers.
// Note that the deregistration method should have been added the the worker's
// tomb cleanup method - stopping the worker cleanly is enough to deregister.
func (r *Resident) cleanupWorkers() error {
	var errs []string
	for id, w := range r.workers {
		if err := worker.Stop(w); err != nil {
			errs = append(errs, errors.Annotatef(err, "worker %d", id).Error())
		}
	}

	if len(errs) != 0 {
		return errors.Errorf("worker cleanup errors:\n\t%s", strings.Join(errs, "\n\t"))
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

func (r *Resident) isStale() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stale
}

func (r *Resident) setStale(stale bool) {
	r.mu.Lock()
	r.stale = stale
	r.mu.Unlock()
}

// unint64Reverse facilitates sorting of a slice in *descending* order.
type uint64Reverse []uint64

func (p uint64Reverse) Len() int           { return len(p) }
func (p uint64Reverse) Less(i, j int) bool { return p[i] > p[j] }
func (p uint64Reverse) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
