// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"container/list"
	"errors"
	"reflect"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

// Watcher watches any changes to the state.
type Watcher struct {
	all *StoreManager

	// The following fields are maintained by the StoreManager
	// goroutine.
	revno   int64
	stopped bool
}

// NewWatcher creates a new watcher that can observe
// changes to an underlying store manager.
func NewWatcher(all *StoreManager) *Watcher {
	return &Watcher{
		all: all,
	}
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	select {
	case w.all.request <- &request{w: w}:
		return nil
	case <-w.all.tomb.Dead():
	}
	return w.all.tomb.Err()
}

var ErrWatcherStopped = errors.New("watcher was stopped")

// Next retrieves all changes that have happened since the last
// time it was called, blocking until there are some changes available.
func (w *Watcher) Next() ([]params.Delta, error) {
	req := &request{
		w:     w,
		reply: make(chan bool),
	}
	select {
	case w.all.request <- req:
	case <-w.all.tomb.Dead():
		err := w.all.tomb.Err()
		if err == nil {
			err = errors.New("shared state watcher was stopped")
		}
		return nil, err
	}
	if ok := <-req.reply; !ok {
		return nil, ErrWatcherStopped
	}
	return req.changes, nil
}

// StoreManager holds a shared record of current state and replies to
// requests from Watchers to tell them when it changes.
type StoreManager struct {
	tomb tomb.Tomb

	// backing knows how to fetch information from
	// the underlying state.
	backing Backing

	// request receives requests from Watcher clients.
	request chan *request

	// all holds information on everything the StoreManager cares about.
	all *Store

	// Each entry in the waiting map holds a linked list of Next requests
	// outstanding for the associated Watcher.
	waiting map[*Watcher]*request
}

// InfoId holds an identifier for an Info item held in a Store.
type InfoId interface{}

// Backing is the interface required by the StoreManager to access the
// underlying state.
type Backing interface {

	// GetAll retrieves information about all information
	// known to the Backing and stashes it in the Store.
	GetAll(all *Store) error

	// Changed informs the backing about a change received
	// from a watcher channel.  The backing is responsible for
	// updating the Store to reflect the change.
	Changed(all *Store, change watcher.Change) error

	// Watch watches for any changes and sends them
	// on the given channel.
	Watch(in chan<- watcher.Change)

	// Unwatch stops watching for changes on the
	// given channel.
	Unwatch(in chan<- watcher.Change)
}

// request holds a message from the Watcher to the
// StoreManager for some changes. The request will be
// replied to when some changes are available.
type request struct {
	// w holds the Watcher that has originated the request.
	w *Watcher

	// reply receives a message when deltas are ready.  If reply is
	// nil, the Watcher will be stopped.  If the reply is true,
	// the request has been processed; if false, the Watcher
	// has been stopped,
	reply chan bool

	// On reply, changes will hold changes that have occurred since
	// the last replied-to Next request.
	changes []params.Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// StoreManager goroutine.
	next *request
}

// newStoreManagerNoRun creates the store manager
// but does not start its run loop.
func newStoreManagerNoRun(backing Backing) *StoreManager {
	return &StoreManager{
		backing: backing,
		request: make(chan *request),
		all:     NewStore(),
		waiting: make(map[*Watcher]*request),
	}
}

// NewStoreManager returns a new StoreManager that retrieves information
// using the given backing.
func NewStoreManager(backing Backing) *StoreManager {
	sm := newStoreManagerNoRun(backing)
	go func() {
		defer sm.tomb.Done()
		// TODO(rog) distinguish between temporary and permanent errors:
		// if we get an error in loop, this logic kill the state's StoreManager
		// forever. This currently fits the way we go about things,
		// because we reconnect to the state on any error, but
		// perhaps there are errors we could recover from.
		sm.tomb.Kill(sm.loop())
	}()
	return sm
}

func (sm *StoreManager) loop() error {
	in := make(chan watcher.Change)
	sm.backing.Watch(in)
	defer sm.backing.Unwatch(in)
	// We have no idea what changes the watcher might be trying to
	// send us while getAll proceeds, but we don't mind, because
	// StoreManager.changed is idempotent with respect to both updates
	// and removals.
	// TODO(rog) Perhaps find a way to avoid blocking all other
	// watchers while GetAll is running.
	if err := sm.backing.GetAll(sm.all); err != nil {
		return err
	}
	for {
		select {
		case <-sm.tomb.Dying():
			return tomb.ErrDying
		case change := <-in:
			if err := sm.backing.Changed(sm.all, change); err != nil {
				return err
			}
		case req := <-sm.request:
			sm.handle(req)
		}
		sm.respond()
	}
}

// Stop stops the StoreManager.
func (sm *StoreManager) Stop() error {
	sm.tomb.Kill(nil)
	return sm.tomb.Wait()
}

// handle processes a request from a Watcher to the StoreManager.
func (sm *StoreManager) handle(req *request) {
	if req.w.stopped {
		// The watcher has previously been stopped.
		if req.reply != nil {
			req.reply <- false
		}
		return
	}
	if req.reply == nil {
		// This is a request to stop the watcher.
		for req := sm.waiting[req.w]; req != nil; req = req.next {
			req.reply <- false
		}
		delete(sm.waiting, req.w)
		req.w.stopped = true
		sm.leave(req.w)
		return
	}
	// Add request to head of list.
	req.next = sm.waiting[req.w]
	sm.waiting[req.w] = req
}

// respond responds to all outstanding requests that are satisfiable.
func (sm *StoreManager) respond() {
	for w, req := range sm.waiting {
		revno := w.revno
		changes := sm.all.ChangesSince(revno)
		if len(changes) == 0 {
			continue
		}
		req.changes = changes
		w.revno = sm.all.latestRevno
		req.reply <- true
		if req := req.next; req == nil {
			// Last request for this watcher.
			delete(sm.waiting, w)
		} else {
			sm.waiting[w] = req
		}
		sm.seen(revno)
	}
}

// seen states that a Watcher has just been given information about
// all entities newer than the given revno.  We assume it has already
// seen all the older entities.
func (sm *StoreManager) seen(revno int64) {
	for e := sm.all.list.Front(); e != nil; {
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
			sm.all.decRef(entry)
		}
		e = next
	}
}

// leave is called when the given watcher leaves.  It decrements the reference
// counts of any entities that have been seen by the watcher.
func (sm *StoreManager) leave(w *Watcher) {
	for e := sm.all.list.Front(); e != nil; {
		next := e.Next()
		entry := e.Value.(*entityEntry)
		if entry.creationRevno <= w.revno {
			// The watcher has seen this entry.
			if entry.removed && entry.revno <= w.revno {
				// The entity has been removed and the
				// watcher has already been informed of that,
				// so its refcount has already been decremented.
				e = next
				continue
			}
			sm.all.decRef(entry)
		}
		e = next
	}
}

// entityEntry holds an entry in the linked list of all entities known
// to a Watcher.
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
	// the ref count is decremented whenever a Watcher that
	// has previously seen the entry now sees that it has been removed;
	// the entry will be deleted when all such Watchers have
	// been notified.
	refCount int

	// info holds the actual information on the entity.
	info params.EntityInfo
}

// Store holds a list of all entities known
// to a Watcher.
type Store struct {
	latestRevno int64
	entities    map[InfoId]*list.Element
	list        *list.List
}

// NewStore returns an Store instance holding information about the
// current state of all entities in the environment.
// It is only exposed here for testing purposes.
func NewStore() *Store {
	all := &Store{
		entities: make(map[InfoId]*list.Element),
		list:     list.New(),
	}
	return all
}

// All returns all the entities stored in the Store,
// oldest first. It is only exposed for testing purposes.
func (a *Store) All() []params.EntityInfo {
	entities := make([]params.EntityInfo, 0, a.list.Len())
	for e := a.list.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		if entry.removed {
			continue
		}
		entities = append(entities, entry.info)
	}
	return entities
}

// add adds a new entity with the given id and associated
// information to the list.
func (a *Store) add(id InfoId, info params.EntityInfo) {
	if a.entities[id] != nil {
		panic("adding new entry with duplicate id")
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
func (a *Store) decRef(entry *entityEntry) {
	if entry.refCount--; entry.refCount > 0 {
		return
	}
	if entry.refCount < 0 {
		panic("negative reference count")
	}
	if !entry.removed {
		return
	}
	id := entry.info.EntityId()
	elem := a.entities[id]
	if elem == nil {
		panic("delete of non-existent entry")
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// delete deletes the entry with the given info id.
func (a *Store) delete(id params.EntityId) {
	elem := a.entities[id]
	if elem == nil {
		return
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// Remove marks that the entity with the given id has
// been removed from the backing. If nothing has seen the
// entity, then we delete it immediately.
func (a *Store) Remove(id params.EntityId) {
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
func (a *Store) Update(info params.EntityInfo) {
	id := info.EntityId()
	elem := a.entities[id]
	if elem == nil {
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
	a.list.MoveToFront(elem)
}

// Get returns the stored entity with the given
// id, or nil if none was found. The contents of the returned entity
// should not be changed.
func (a *Store) Get(id params.EntityId) params.EntityInfo {
	if e := a.entities[id]; e != nil {
		return e.Value.(*entityEntry).info
	}
	return nil
}

// ChangesSince returns any changes that have occurred since
// the given revno, oldest first.
func (a *Store) ChangesSince(revno int64) []params.Delta {
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
	changes := make([]params.Delta, 0, n)
	for ; e != nil; e = e.Prev() {
		entry := e.Value.(*entityEntry)
		if entry.removed && entry.creationRevno > revno {
			// Don't include entries that have been created
			// and removed since the revno.
			continue
		}
		changes = append(changes, params.Delta{
			Removed: entry.removed,
			Entity:  entry.info,
		})
	}
	return changes
}
