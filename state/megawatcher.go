package state

import (
	"container/list"
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"reflect"
)

// StateWatcher watches any changes to the state.
// TODO(rog) rename this to StateWatcher when allWatcher is complete.
type StateWatcher struct {
	all *allWatcher
	// The following fields are maintained by the allWatcher
	// goroutine.
	revno   int64
	stopped bool
}

func newStateWatcher(st *State) *StateWatcher {
	return &StateWatcher{
		all: st.allWatcher,
	}
}

// Stop stops the watcher.
func (w *StateWatcher) Stop() error {
	select {
	case w.all.request <- &allRequest{w: w}:
		return nil
	case <-w.all.tomb.Dead():
	}
	return w.all.tomb.Err()
}

var errWatcherStopped = errors.New("state watcher was stopped")

// Next retrieves all changes that have happened since the last
// time it was called, blocking until there are some changes available.
func (w *StateWatcher) Next() ([]params.Delta, error) {
	req := &allRequest{
		w:     w,
		reply: make(chan bool),
	}
	select {
	case w.all.request <- req:
	case <-w.all.tomb.Dead():
		err := w.all.tomb.Err()
		if err == nil {
			err = errWatcherStopped
		}
		return nil, err
	}
	if ok := <-req.reply; !ok {
		// TODO better error?
		return nil, errWatcherStopped
	}
	return req.changes, nil
}

// allWatcher holds a shared record of all current state and replies to
// requests from StateWatches to tell them when it changes.
// TODO(rog) complete this type and its methods.
type allWatcher struct {
	tomb tomb.Tomb

	// backing knows how to fetch information from
	// the underlying state.
	backing allWatcherBacking

	// request receives requests from StateWatcher clients.
	request chan *allRequest

	// all holds information on everything the allWatcher cares about.
	all *allInfo

	// Each entry in the waiting map holds a linked list of Next requests
	// outstanding for the associated StateWatcher.
	waiting map[*StateWatcher]*allRequest
}

// allWatcherBacking is the interface required
// by the allWatcher to access the underlying state.
// It is an interface for testing purposes.
// TODO(rog) complete this type and its methods.
type allWatcherBacking interface {
	// entityIdForInfo returns the entity id corresponding
	// to the given entity info.
	entityIdForInfo(info params.EntityInfo) entityId

	// getAll retrieves information about all known entities in the state
	// into the given allInfo.
	getAll(all *allInfo) error

	// changed informs the backing about a change to the entity with
	// the given id.  The backing is responsible for updating the
	// allInfo to reflect the change.
	changed(all *allInfo, id entityId) error

	// watch watches for any changes and sends them
	// on the given channel.
	watch(in chan<- watcher.Change)

	// unwatch stops watching for changes on the
	// given channel.
	unwatch(in chan<- watcher.Change)
}

// entityId holds the mongo identifier of an entity.
type entityId struct {
	collection string
	id         interface{}
}

// allRequest holds a request from the StateWatcher to the
// allWatcher for some changes. The request will be
// replied to when some changes are available.
type allRequest struct {
	// w holds the StateWatcher that has originated the request.
	w *StateWatcher

	// reply receives a message when deltas are ready.  If reply is
	// nil, the StateWatcher will be stopped.  If the reply is true,
	// the request has been processed; if false, the StateWatcher
	// has been stopped,
	reply chan bool

	// On reply, changes will hold changes that have occurred since
	// the last replied-to Next request.
	changes []params.Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// allWatcher goroutine.
	next *allRequest
}

// newAllWatcher returns a new allWatcher that retrieves information
// using the given backing. It does not start it running.
func newAllWatcher(backing allWatcherBacking) *allWatcher {
	return &allWatcher{
		backing: backing,
		request: make(chan *allRequest),
		all:     newAllInfo(),
		waiting: make(map[*StateWatcher]*allRequest),
	}
}

func (aw *allWatcher) run() {
	defer aw.tomb.Done()
	// TODO(rog) distinguish between temporary and permanent errors:
	// if we get an error in loop, this logic kill the state's allWatcher
	// forever. This currently fits the way we go about things,
	// because we reconnect to the state on any error, but
	// perhaps there are errors we could recover from.
	aw.tomb.Kill(aw.loop())
}

func (aw *allWatcher) loop() error {
	in := make(chan watcher.Change)
	aw.backing.watch(in)
	defer aw.backing.unwatch(in)
	// We have no idea what changes the watcher might be trying to
	// send us while getAll proceeds, but we don't mind, because
	// allWatcher.changed is idempotent with respect to both updates
	// and removals.
	// TODO(rog) Perhaps find a way to avoid blocking all other
	// watchers while getAll is running.
	if err := aw.backing.getAll(aw.all); err != nil {
		return err
	}
	for {
		select {
		case <-aw.tomb.Dying():
			return tomb.ErrDying
		case change := <-in:
			id := entityId{
				collection: change.C,
				id:         change.Id,
			}
			if err := aw.backing.changed(aw.all, id); err != nil {
				return err
			}
		case req := <-aw.request:
			aw.handle(req)
		}
		aw.respond()
	}
	panic("unreachable")
}

// Stop stops the allWatcher.
func (aw *allWatcher) Stop() error {
	aw.tomb.Kill(nil)
	return aw.tomb.Wait()
}

// handle processes a request from a StateWatcher to the allWatcher.
func (aw *allWatcher) handle(req *allRequest) {
	if req.w.stopped {
		// The watcher has previously been stopped.
		if req.reply != nil {
			req.reply <- false
		}
		return
	}
	if req.reply == nil {
		// This is a request to stop the watcher.
		for req := aw.waiting[req.w]; req != nil; req = req.next {
			req.reply <- false
		}
		delete(aw.waiting, req.w)
		req.w.stopped = true
		aw.leave(req.w)
		return
	}
	// Add request to head of list.
	req.next = aw.waiting[req.w]
	aw.waiting[req.w] = req
}

// respond responds to all outstanding requests that are satisfiable.
func (aw *allWatcher) respond() {
	for w, req := range aw.waiting {
		revno := w.revno
		changes := aw.all.changesSince(revno)
		if len(changes) == 0 {
			continue
		}
		req.changes = changes
		w.revno = aw.all.latestRevno
		req.reply <- true
		if req := req.next; req == nil {
			// Last request for this watcher.
			delete(aw.waiting, w)
		} else {
			aw.waiting[w] = req
		}
		aw.seen(revno)
	}
}

// seen states that a StateWatcher has just been given information about
// all entities newer than the given revno.  We assume it has already
// seen all the older entities.
func (aw *allWatcher) seen(revno int64) {
	for e := aw.all.list.Front(); e != nil; {
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
			aw.all.decRef(entry, aw.backing.entityIdForInfo(entry.info))
		}
		e = next
	}
}

// leave is called when the given watcher leaves.  It decrements the reference
// counts of any entities that have been seen by the watcher.
func (aw *allWatcher) leave(w *StateWatcher) {
	for e := aw.all.list.Front(); e != nil; {
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
			aw.all.decRef(entry, aw.backing.entityIdForInfo(entry.info))
		}
		e = next
	}
}

// allWatcherStateBacking implements allWatcherBacking by
// fetching entities from the State.
type allWatcherStateBacking struct {
	st *State
	// collections
	collectionByName map[string]allWatcherStateCollection
	collectionByKind map[string]allWatcherStateCollection
}

// allWatcherStateCollection holds information about a
// collection watched by an allWatcher and the
// type of value we use to store entity information
// for that collection.
type allWatcherStateCollection struct {
	*mgo.Collection
	// infoSliceType stores the type of a slice of the info type
	// that we use for this collection.  In Go 1.1 we can change
	// this to use the type itself, as we'll have reflect.SliceOf.
	infoSliceType reflect.Type
}

func newAllWatcherStateBacking(st *State) allWatcherBacking {
	b := &allWatcherStateBacking{
		st:               st,
		collectionByName: make(map[string]allWatcherStateCollection),
		collectionByKind: make(map[string]allWatcherStateCollection),
	}
	collections := []allWatcherStateCollection{{
		Collection:    st.machines,
		infoSliceType: reflect.TypeOf([]params.MachineInfo(nil)),
	}, {
		Collection:    st.units,
		infoSliceType: reflect.TypeOf([]params.UnitInfo(nil)),
	}, {
		Collection:    st.services,
		infoSliceType: reflect.TypeOf([]params.ServiceInfo(nil)),
	}, {
		Collection:    st.relations,
		infoSliceType: reflect.TypeOf([]params.RelationInfo(nil)),
	}, {
		Collection:    st.annotations,
		infoSliceType: reflect.TypeOf([]params.AnnotationInfo(nil)),
	}}
	// Populate the collection maps from the above set of collections.
	for _, c := range collections {
		// Create a new instance of the info type so we can
		// find out its kind.
		info := reflect.New(c.infoSliceType.Elem()).Interface().(params.EntityInfo)
		kind := info.EntityKind()
		if _, ok := b.collectionByKind[kind]; ok {
			panic(fmt.Errorf("duplicate collection kind %q", kind))
		}
		b.collectionByKind[kind] = c
		if _, ok := b.collectionByName[c.Name]; ok {
			panic(fmt.Errorf("duplicate collection name %q", kind))
		}
		b.collectionByName[c.Name] = c
	}
	return b
}

// watch watches all the collections.
func (b *allWatcherStateBacking) watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.WatchCollection(c.Name, in)
	}
}

// watch unwatches all the collections.
func (b *allWatcherStateBacking) unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.UnwatchCollection(c.Name, in)
	}
}

// getAll fetches all items that we want to watch from the state.
func (b *allWatcherStateBacking) getAll(all *allInfo) error {
	// TODO(rog) fetch collections concurrently?
	for _, c := range b.collectionByName {
		infoSlicePtr := reflect.New(c.infoSliceType).Interface()
		if err := c.Find(nil).All(infoSlicePtr); err != nil {
			return fmt.Errorf("cannot get all %s: %v", c.Name, err)
		}
		infos := reflect.ValueOf(infoSlicePtr).Elem()
		for i := 0; i < infos.Len(); i++ {
			info := infos.Index(i).Addr().Interface().(params.EntityInfo)
			all.add(b.entityIdForInfo(info), info)
		}
	}
	return nil
}

// changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherStateBacking) changed(all *allInfo, id entityId) error {
	// TODO(rog) investigate ways that this can be made more efficient
	// than simply fetching each entity in turn.
	info, err := b.fetch(id)
	if err == mgo.ErrNotFound {
		all.markRemoved(id)
		return nil
	}
	if err != nil {
		return err
	}
	all.update(id, info)
	return nil
}

func (b *allWatcherStateBacking) fetch(id entityId) (params.EntityInfo, error) {
	c, ok := b.collectionByName[id.collection]
	if !ok {
		panic(fmt.Errorf("unknown collection %q in fetch request", id.collection))
	}
	info := reflect.New(c.infoSliceType.Elem()).Interface().(params.EntityInfo)
	if err := c.FindId(id.id).One(info); err != nil {
		return nil, err
	}
	return info, nil
}

// entityIdForInfo returns the entity id of the given entity document.
func (b *allWatcherStateBacking) entityIdForInfo(info params.EntityInfo) entityId {
	c, ok := b.collectionByKind[info.EntityKind()]
	if !ok {
		panic(fmt.Errorf("entity with unknown kind %q", info.EntityKind()))
	}
	return entityId{
		collection: c.Name,
		id:         info.EntityId(),
	}
}

// entityEntry holds an entry in the linked list of all entities known
// to a StateWatcher.
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
	// the ref count is decremented whenever a StateWatcher that
	// has previously seen the entry now sees that it has been removed;
	// the entry will be deleted when all such StateWatchers have
	// been notified.
	refCount int

	// info holds the actual information on the entity.
	info params.EntityInfo
}

// allInfo holds a list of all entities known
// to a StateWatcher.
type allInfo struct {
	latestRevno int64
	entities    map[entityId]*list.Element
	list        *list.List
}

// newAllInfo returns an allInfo instance holding information about the
// current state of all entities in the environment.
func newAllInfo() *allInfo {
	all := &allInfo{
		entities: make(map[entityId]*list.Element),
		list:     list.New(),
	}
	return all
}

// add adds a new entity with the given id and associated
// information to the list.
func (a *allInfo) add(id entityId, info params.EntityInfo) {
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
func (a *allInfo) decRef(entry *entityEntry, id entityId) {
	if entry.refCount--; entry.refCount > 0 {
		return
	}
	if entry.refCount < 0 {
		panic("negative reference count")
	}
	if !entry.removed {
		return
	}
	elem := a.entities[id]
	if elem == nil {
		panic("delete of non-existent entry")
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// delete deletes the entry with the given entity id.
func (a *allInfo) delete(id entityId) {
	elem := a.entities[id]
	if elem == nil {
		return
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// markRemoved marks that the entity with the given id has
// been removed from the state. If nothing has seen the
// entity, then we delete it immediately.
func (a *allInfo) markRemoved(id entityId) {
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

// update updates the information for the entity with
// the given id.
func (a *allInfo) update(id entityId, info params.EntityInfo) {
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

// changesSince returns any changes that have occurred since
// the given revno, oldest first.
func (a *allInfo) changesSince(revno int64) []params.Delta {
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
