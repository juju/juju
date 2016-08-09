// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"container/list"
	stderrors "errors"
	"reflect"

	"github.com/juju/errors"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

// Multiwatcher watches any changes to the state.
type Multiwatcher struct {
	all *storeManager

	// used indicates that the watcher was used (i.e. Next() called).
	used bool

	// The following fields are maintained by the storeManager
	// goroutine.
	revno   int64
	stopped bool
}

// NewMultiwatcher creates a new watcher that can observe
// changes to an underlying store manager.
func NewMultiwatcher(all *storeManager) *Multiwatcher {
	// Note that we want to be clear about the defaults. So we set zero
	// values explicitly.
	//  used:    false means that the watcher has not been used yet
	//  revno:   0 means that *all* transactions prior to the first
	//           Next() call will be reflected in the deltas.
	//  stopped: false means that the watcher immediately starts off
	//           handling changes.
	return &Multiwatcher{
		all:     all,
		used:    false,
		revno:   0,
		stopped: false,
	}
}

// Stop stops the watcher.
func (w *Multiwatcher) Stop() error {
	select {
	case w.all.request <- &request{w: w}:
		return nil
	case <-w.all.tomb.Dead():
	}
	return errors.Trace(w.all.tomb.Err())
}

var ErrStopped = stderrors.New("watcher was stopped")

// Next retrieves all changes that have happened since the last
// time it was called, blocking until there are some changes available.
//
// The result from the initial call to Next() is different from
// subsequent calls. The latter will reflect changes that have happened
// since the last Next() call. In contrast, the initial Next() call will
// return the deltas that represent the model's complete state at that
// moment, even when the model is empty. In that empty model case an
// empty set of deltas is returned.
func (w *Multiwatcher) Next() ([]multiwatcher.Delta, error) {
	req := &request{
		w:     w,
		reply: make(chan bool),
	}
	if !w.used {
		req.noChanges = make(chan struct{})
		w.used = true
	}

	select {
	case <-w.all.tomb.Dying():
		return nil, errors.Errorf("shared state watcher was stopped")
	case w.all.request <- req:
	}

	// TODO(ericsnow) Clean up Multiwatcher/storeManager interaction.
	// Relying on req.reply and req.noChanges here is not an ideal
	// solution. It reflects the level of coupling we have between
	// the Multiwatcher, request, and storeManager types.
	select {
	case <-w.all.tomb.Dying():
		return nil, errors.Errorf("shared state watcher was stopped")
	case ok := <-req.reply:
		if !ok {
			return nil, errors.Trace(ErrStopped)
		}
	case <-req.noChanges:
		return []multiwatcher.Delta{}, nil
	}
	return req.changes, nil
}

// storeManager holds a shared record of current state and replies to
// requests from Multiwatchers to tell them when it changes.
type storeManager struct {
	tomb tomb.Tomb

	// backing knows how to fetch information from
	// the underlying state.
	backing Backing

	// request receives requests from Multiwatcher clients.
	request chan *request

	// all holds information on everything the storeManager cares about.
	all *multiwatcherStore

	// Each entry in the waiting map holds a linked list of Next requests
	// outstanding for the associated Multiwatcher.
	waiting map[*Multiwatcher]*request
}

// Backing is the interface required by the storeManager to access the
// underlying state.
type Backing interface {

	// GetAll retrieves information about all information
	// known to the Backing and stashes it in the Store.
	GetAll(all *multiwatcherStore) error

	// Changed informs the backing about a change received
	// from a watcher channel.  The backing is responsible for
	// updating the Store to reflect the change.
	Changed(all *multiwatcherStore, change watcher.Change) error

	// Watch watches for any changes and sends them
	// on the given channel.
	Watch(in chan<- watcher.Change)

	// Unwatch stops watching for changes on the
	// given channel.
	Unwatch(in chan<- watcher.Change)

	// Release cleans up resources opened by the Backing.
	Release() error
}

// request holds a message from the Multiwatcher to the
// storeManager for some changes. The request will be
// replied to when some changes are available.
type request struct {
	// w holds the Multiwatcher that has originated the request.
	w *Multiwatcher

	// reply receives a message when deltas are ready.  If reply is
	// nil, the Multiwatcher will be stopped.  If the reply is true,
	// the request has been processed; if false, the Multiwatcher
	// has been stopped,
	reply chan bool

	// noChanges receives a message when the manager checks for changes
	// and there are none.
	noChanges chan struct{}

	// On reply, changes will hold changes that have occurred since
	// the last replied-to Next request.
	changes []multiwatcher.Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// storeManager goroutine.
	next *request
}

// newStoreManagerNoRun creates the store manager
// but does not start its run loop.
func newStoreManagerNoRun(backing Backing) *storeManager {
	return &storeManager{
		backing: backing,
		request: make(chan *request),
		all:     newStore(),
		waiting: make(map[*Multiwatcher]*request),
	}
}

// newStoreManager returns a new storeManager that retrieves information
// using the given backing.
func newStoreManager(backing Backing) *storeManager {
	sm := newStoreManagerNoRun(backing)
	go func() {
		defer sm.tomb.Done()
		// TODO(rog) distinguish between temporary and permanent errors:
		// if we get an error in loop, this logic kill the state's storeManager
		// forever. This currently fits the way we go about things,
		// because we reconnect to the state on any error, but
		// perhaps there are errors we could recover from.

		err := sm.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("store manager loop failed: %v", err)
		}
		sm.tomb.Kill(cause)
	}()
	return sm
}

func (sm *storeManager) loop() error {
	in := make(chan watcher.Change)
	sm.backing.Watch(in)
	defer sm.backing.Unwatch(in)
	// We have no idea what changes the watcher might be trying to
	// send us while getAll proceeds, but we don't mind, because
	// storeManager.changed is idempotent with respect to both updates
	// and removals.
	// TODO(rog) Perhaps find a way to avoid blocking all other
	// watchers while GetAll is running.
	if err := sm.backing.GetAll(sm.all); err != nil {
		return err
	}
	for {
		select {
		case <-sm.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case change := <-in:
			if err := sm.backing.Changed(sm.all, change); err != nil {
				return errors.Trace(err)
			}
		case req := <-sm.request:
			sm.handle(req)
		}
		sm.respond()
	}
}

// Stop stops the storeManager.
func (sm *storeManager) Stop() error {
	sm.tomb.Kill(nil)
	return errors.Trace(sm.tomb.Wait())
}

// handle processes a request from a Multiwatcher to the storeManager.
func (sm *storeManager) handle(req *request) {
	if req.w.stopped {
		// The watcher has previously been stopped.
		if req.reply != nil {
			select {
			case req.reply <- false:
			case <-sm.tomb.Dying():
			}
		}
		return
	}
	if req.reply == nil {
		// This is a request to stop the watcher.
		for req := sm.waiting[req.w]; req != nil; req = req.next {
			select {
			case req.reply <- false:
			case <-sm.tomb.Dying():
			}
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
func (sm *storeManager) respond() {
	for w, req := range sm.waiting {
		revno := w.revno
		changes := sm.all.ChangesSince(revno)
		if len(changes) == 0 {
			if req.noChanges != nil {
				req.noChanges <- struct{}{}
				sm.removeWaitingReq(w, req)
			}
			continue
		}

		req.changes = changes
		w.revno = sm.all.latestRevno
		req.reply <- true
		sm.removeWaitingReq(w, req)
		sm.seen(revno)
	}
}

func (sm *storeManager) removeWaitingReq(w *Multiwatcher, req *request) {
	if req := req.next; req == nil {
		// Last request for this watcher.
		delete(sm.waiting, w)
	} else {
		sm.waiting[w] = req
	}
}

// seen states that a Multiwatcher has just been given information about
// all entities newer than the given revno.  We assume it has already
// seen all the older entities.
func (sm *storeManager) seen(revno int64) {
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
func (sm *storeManager) leave(w *Multiwatcher) {
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
// to a Multiwatcher.
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
	info multiwatcher.EntityInfo
}

// multiwatcherStore holds a list of all entities known
// to a Multiwatcher.
type multiwatcherStore struct {
	latestRevno int64
	entities    map[interface{}]*list.Element
	list        *list.List
}

// newStore returns an Store instance holding information about the
// current state of all entities in the model.
// It is only exposed here for testing purposes.
func newStore() *multiwatcherStore {
	return &multiwatcherStore{
		entities: make(map[interface{}]*list.Element),
		list:     list.New(),
	}
}

// All returns all the entities stored in the Store,
// oldest first. It is only exposed for testing purposes.
func (a *multiwatcherStore) All() []multiwatcher.EntityInfo {
	entities := make([]multiwatcher.EntityInfo, 0, a.list.Len())
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
func (a *multiwatcherStore) add(id interface{}, info multiwatcher.EntityInfo) {
	if _, ok := a.entities[id]; ok {
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
func (a *multiwatcherStore) decRef(entry *entityEntry) {
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
	elem, ok := a.entities[id]
	if !ok {
		panic("delete of non-existent entry")
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// delete deletes the entry with the given info id.
func (a *multiwatcherStore) delete(id multiwatcher.EntityId) {
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
func (a *multiwatcherStore) Remove(id multiwatcher.EntityId) {
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
func (a *multiwatcherStore) Update(info multiwatcher.EntityInfo) {
	id := info.EntityId()
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
	a.list.MoveToFront(elem)
}

// Get returns the stored entity with the given id, or nil if none was found.
// The contents of the returned entity MUST not be changed.
func (a *multiwatcherStore) Get(id multiwatcher.EntityId) multiwatcher.EntityInfo {
	e, ok := a.entities[id]
	if !ok {
		return nil
	}
	return e.Value.(*entityEntry).info
}

// ChangesSince returns any changes that have occurred since
// the given revno, oldest first.
func (a *multiwatcherStore) ChangesSince(revno int64) []multiwatcher.Delta {
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
	changes := make([]multiwatcher.Delta, 0, n)
	for ; e != nil; e = e.Prev() {
		entry := e.Value.(*entityEntry)
		if entry.removed && entry.creationRevno > revno {
			// Don't include entries that have been created
			// and removed since the revno.
			continue
		}
		changes = append(changes, multiwatcher.Delta{
			Removed: entry.removed,
			Entity:  entry.info,
		})
	}
	return changes
}
