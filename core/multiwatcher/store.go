// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"container/list"
	"reflect"
	"sync"

	"github.com/kr/pretty"

	"github.com/juju/juju/core/logger"
)

// Store stores the current entities to use as a basis for the multiwatcher
// notifications.
type Store interface {
	All() []EntityInfo
	// ChangesSince takes revno. A zero implies that this is the first call for changes.
	// A slice of changes is returned along with the latest revno that the store has seen.
	ChangesSince(revno int64) ([]Delta, int64)

	// AddReference and DecReference are used for internal reference counting for the
	// watchers that have been notified.
	// TODO: determine if this is actually useful, and whether this is the right place for it.
	AddReference(revno int64)
	DecReference(revno int64)

	Get(id EntityID) EntityInfo
	Update(info EntityInfo)
	Remove(id EntityID)

	// Size returns the internal size of the store's list.
	// Used only for tests and metrics.
	Size() int
}

// entityEntry holds an entry in the linked list of all entities known
// to a params.
type entityEntry struct {
	// The revno holds the local idea of the latest change to the
	// given entity.  It is not the same as the transaction revno -
	// this means we can unconditionally move a newly fetched entity
	// to the front of the list without worrying if the revno has
	// changed since the watcher reported it.
	revno int64

	// creationRevno holds the revision number when the
	// entity was created.
	creationRevno int64

	// removed marks whether the entity has been removed.
	removed bool

	// refCount holds a count of the number of watchers that
	// have seen this entity. When the entity is marked as removed,
	// the ref count is decremented whenever a Multiwatcher that
	// has previously seen the entry now sees that it has been removed;
	// the entry will be deleted when all such Multiwatchers have
	// been notified.
	refCount int

	// info holds the actual information on the entity.
	info EntityInfo
}

// store holds a list of all known entities.
type store struct {
	mu          sync.Mutex
	latestRevno int64
	entities    map[interface{}]*list.Element
	list        *list.List
	logger      logger.Logger
}

// NewStore returns an Store instance holding information about the
// current state of all entities in the model.
// It is only exposed here for testing purposes.
func NewStore(logger logger.Logger) Store {
	return newStore(logger)
}

func newStore(logger logger.Logger) *store {
	return &store{
		entities: make(map[interface{}]*list.Element),
		list:     list.New(),
		logger:   logger,
	}
}

// Size returns the length of the internal list.
func (a *store) Size() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.list.Len()
}

// All returns all the entities stored in the Store,
// oldest first.
func (a *store) All() []EntityInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	entities := make([]EntityInfo, 0, a.list.Len())
	for e := a.list.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		if entry.removed {
			continue
		}
		entities = append(entities, entry.info.Clone())
	}
	return entities
}

// add adds a new entity with the given id and associated
// information to the list.
func (a *store) add(id interface{}, info EntityInfo) {
	if _, ok := a.entities[id]; ok {
		a.logger.Criticalf("programming error: adding new entry with duplicate id %q", id)
		return
	}
	a.latestRevno++
	entry := &entityEntry{
		info:          info,
		revno:         a.latestRevno,
		creationRevno: a.latestRevno,
	}
	a.entities[id] = a.list.PushFront(entry)
}

// decRef decrements the reference count of an entry within the list,
// deleting it if it becomes zero and the entry is removed.
func (a *store) decRef(entry *entityEntry) {
	if entry.refCount--; entry.refCount > 0 {
		return
	}
	if entry.refCount < 0 {
		a.logger.Criticalf("programming error: negative reference count\n%s", pretty.Sprint(entry))
		return
	}
	if !entry.removed {
		return
	}
	id := entry.info.EntityID()
	elem, ok := a.entities[id]
	if !ok {
		a.logger.Criticalf("programming error: delete of non-existent entry\n%s", pretty.Sprint(entry))
		return
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// delete deletes the entry with the given info id.
func (a *store) delete(id EntityID) {
	elem, ok := a.entities[id]
	if !ok {
		return
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// Remove marks that the entity with the given id has
// been removed from the backing. If nothing has seen the
// entity, then we delete it immediately.
func (a *store) Remove(id EntityID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if elem := a.entities[id]; elem != nil {
		entry := elem.Value.(*entityEntry)
		if entry.removed {
			return
		}
		a.latestRevno++
		if entry.refCount == 0 {
			a.delete(id)
			return
		}
		entry.revno = a.latestRevno
		entry.removed = true
		a.list.MoveToFront(elem)
	}
}

// Update updates the information for the given entity.
func (a *store) Update(info EntityInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := info.EntityID()
	elem, ok := a.entities[id]
	if !ok {
		a.add(id, info)
		return
	}
	entry := elem.Value.(*entityEntry)
	// Nothing has changed, so change nothing.
	// TODO(rog) do the comparison more efficiently.
	if reflect.DeepEqual(info, entry.info) {
		return
	}
	// We already know about the entity; update its doc.
	a.latestRevno++
	entry.revno = a.latestRevno
	entry.info = info
	// The app might have been removed and re-added.
	entry.removed = false
	a.list.MoveToFront(elem)
}

// Get returns the stored entity with the given id, or nil if none was found.
// The contents of the returned entity MUST not be changed.
func (a *store) Get(id EntityID) EntityInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	e, ok := a.entities[id]
	if !ok {
		return nil
	}
	ei := e.Value.(*entityEntry).info
	if ei == nil {
		return nil
	}
	// Always clone to prevent data races/mutating internal store state which will miss
	// sending changes.
	return ei.Clone()
}

// ChangesSince returns any changes that have occurred since
// the given revno, oldest first.
func (a *store) ChangesSince(revno int64) ([]Delta, int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	e := a.list.Front()
	n := 0
	for ; e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
		n++
	}
	if e != nil {
		// We've found an element that we've already seen.
		e = e.Prev()
	} else {
		// We haven't seen any elements, so we want all of them.
		e = a.list.Back()
		n++
	}
	changes := make([]Delta, 0, n)
	for ; e != nil; e = e.Prev() {
		entry := e.Value.(*entityEntry)
		if entry.removed && entry.creationRevno > revno {
			// Don't include entries that have been created
			// and removed since the revno.
			continue
		}
		// Use clone to make a copy to avoid races.
		changes = append(changes, Delta{
			Removed: entry.removed,
			Entity:  entry.info.Clone(),
		})
	}
	return changes, a.latestRevno
}

// AddReference states that a Multiwatcher has just been given information about
// all entities newer than the given revno.  We assume it has already seen all
// the older entities.
func (a *store) AddReference(revno int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for e := a.list.Front(); e != nil; {
		next := e.Next()
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
		if entry.creationRevno > revno {
			if !entry.removed {
				// This is a new entity that hasn't been seen yet,
				// so increment the entry's refCount.
				entry.refCount++
			}
		} else if entry.removed {
			// This is an entity that we previously saw, but
			// has now been removed, so decrement its refCount, removing
			// the entity if nothing else is waiting to be notified that it's
			// gone.
			a.decRef(entry)
		}
		e = next
	}
}

// DecReference is called when a watcher leaves.  It decrements the reference
// counts of any entities that have been seen by the watcher.
func (a *store) DecReference(revno int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for e := a.list.Front(); e != nil; {
		next := e.Next()
		entry := e.Value.(*entityEntry)
		if entry.creationRevno <= revno {
			// The watcher has seen this entry.
			if entry.removed && entry.revno <= revno {
				// The entity has been removed and the
				// watcher has already been informed of that,
				// so its refcount has already been decremented.
				e = next
				continue
			}
			a.decRef(entry)
		}
		e = next
	}
}
