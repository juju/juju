package state

import (
	"container/list"
	"fmt"
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"reflect"
	"sync"
	"time"
)

// StateWatcher watches any changes to the state.
type StateWatcher struct {
	all *allWatcher

	mu  sync.Mutex
	err error

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
	w.mu.Lock()
	err := w.all.tomb.Err()
	w.err = err
	w.mu.Unlock()
	return err
}

// Err returns 
func (w *StateWatcher) Err() error {
	w.mu.Lock()
	err := w.err
	w.mu.Unlock()
	return err
}

// Get retrieves all changes that have happened since the given revision
// number, blocking until there are some changes available.  It also
// returns the revision number of the latest change.
func (w *StateWatcher) Next() ([]Delta, error) {
	req := &allRequest{
		w:     w,
		reply: make(chan bool),
	}
	select {
	case w.all.request <- req:
	case <-w.all.tomb.Dead():
		return nil, w.all.tomb.Err()
	}
	if ok := <-req.reply; !ok {
		// TODO better error
		return nil, fmt.Errorf("state watcher was stopped")
	}
	return req.changes, nil
}

// allWatcher holds a record of all current state and replies to
// requests from StateWatches to tell them when it changes.
type allWatcher struct {
	tomb    tomb.Tomb
	st      *State
	request chan *allRequest
}

func newAllWatcher(st *State) *allWatcher {
	aw := &allWatcher{st: st}
	go func() {
		defer aw.tomb.Done()
		// TODO(rog) distinguish between temporary and permanent errors:
		// if we get an error in loop, this logic kill the state's allWatcher
		// forever. This currently fits the way we go about things,
		// because we reconnect to the state on any error, but
		// perhaps there are errors we could recover from.
		aw.tomb.Kill(aw.loop())
	}()
	return aw
}

// allRequest holds a request from the StateWatcher to the
// allWatcher for some changes. The request will be
// replied to when some changes are available.
type allRequest struct {
	// w holds the StateWatcher that has originated the request.
	w *StateWatcher

	// reply receives a message when deltas are ready.  If it is
	// nil, the watcher will be stopped.
	// If the reply is false, the watcher has been stopped.
	reply chan bool

	// On reply, changes will hold changes that have occurred since
	// the last replied-to next request.
	changes []Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// allWatcher goroutine.
	next *allRequest
}

var idleTimeout = 5 * time.Minute

func (aw *allWatcher) loop() error {
	var all *allInfo
	// Each entry in the map holds a linked list of Next requests
	// outstanding for the associated StateWatcher.
	reqs := make(map[*StateWatcher]*allRequest)

	in := make(chan watcher.Change)
	unwatch := func() {
		if all == nil {
			return
		}
		aw.st.watcher.UnwatchCollection(aw.st.machines.Name, in)
		aw.st.watcher.UnwatchCollection(aw.st.services.Name, in)
		aw.st.watcher.UnwatchCollection(aw.st.units.Name, in)
		aw.st.watcher.UnwatchCollection(aw.st.relations.Name, in)
		all = nil
	}
	defer unwatch()

	var idleTimer *time.Timer
	for {
		var idlec <-chan time.Time
		if idleTimer != nil {
			idlec = idleTimer.C
		}
		select {
		case <-aw.tomb.Dying():
			return tomb.ErrDying
		case change := <-in:
			if err := all.update(entityId{change.C, change.Id}); err != nil {
				return err
			}
		case <-idlec:
			// We've had no watchers for at least idleTimeout duration,
			// so stop watching and bide our time until another
			// request comes along.
			idleTimer = nil
			unwatch()
		case req := <-aw.request:
			if req.w.stopped {
				// The watcher has previously been stopped.
				req.reply <- false
				break
			}
			if req.reply == nil {
				// This is a request to stop the watcher.
				for req := reqs[req.w]; req != nil; req = req.next {
					req.reply <- false
				}
				delete(reqs, req.w)
				aw.leave(all, req.w)
				break
			}
			if idleTimer != nil {
				idleTimer.Stop()
				idleTimer = nil
			}
			// Start watching everything if we are not
			// already doing so.
			if all == nil {
				var err error
				all, err = newAllInfo(aw.st)
				if err != nil {
					return err
				}
				aw.st.watcher.WatchCollection(aw.st.machines.Name, in)
				aw.st.watcher.WatchCollection(aw.st.services.Name, in)
				aw.st.watcher.WatchCollection(aw.st.units.Name, in)
				aw.st.watcher.WatchCollection(aw.st.relations.Name, in)
			}
			// Add request to head of list.
			req.next = reqs[req.w]
			reqs[req.w] = req
		}
		// Something has changed - go through all watchers that
		// have outstanding requests and satisfy them if
		// possible. Because it's very common for many
		// watchers to share the same revno, we categorize
		// the requests by watcher revno, then return the same
		// set of deltas for all watchers with a given revno.
		reqByRevno := make(map[int64][]*allRequest)
		for _, req := range reqs {
			if all.latestRevno > req.w.revno {
				reqByRevno[req.w.revno] = append(reqByRevno[req.w.revno])
			}
		}
		for revno, pendingReqs := range reqByRevno {
			changes := all.changesSince(revno)
			for _, req := range pendingReqs {
				// Reply to request and remove it from pending requests.
				w := req.w
				req.changes = changes
				w.revno = all.latestRevno
				if req := req.next; req == nil {
					// Last request for this watcher.
					delete(reqs, w)
				} else {
					reqs[w] = req
				}
			}
			aw.adjustRefCounts(all, revno, len(reqs))
		}
		// If we have no watchers remaining, start a timer that will
		// tell us to go into idle mode after some while.
		if len(reqs) == 0 {
			idleTimer = time.NewTimer(idleTimeout)
		}
	}
	panic("unreachable")
}

// adjustRefcounts increments the reference counts of all entities
// created since the given revno and decrements the reference counts of
// all entities created before the given revno that have now been
// removed. The amount to increment or decrement by is
// given by n, the number of watchers that share this revno.
func (aw *allWatcher) adjustRefCounts(all *allInfo, revno int64, n int) {
	for e := all.list.Front(); e != nil; {
		prev := e.Prev()
		entry := e.Value.(*entityEntry)
		if entry.creationRevno > revno {
			if !entry.removed {
				// This is a new entity that hasn't been seen yet,
				// so increment the entry's refCount.	
				entry.refCount += n
			}
		} else if entry.removed {
			// This an entity that has already been seen,
			// so decrement its refCount, removing the entry if
			// necessary.
			all.decRef(entry, n)
		}
		e = prev
	}
}

// leave is called when the given watcher leaves.  It decrements the reference
// counts of any entities that have been seen by the watcher.
func (a *allWatcher) leave(all *allInfo, w *StateWatcher) {
	for e := all.list.Front(); e != nil; {
		prev := e.Prev()
		entry := e.Value.(*entityEntry)
		if entry.creationRevno <= w.revno {
			// The watcher has seen this entry.
			if entry.removed && entry.revno <= w.revno {
				// The entity has been removed and the
				// watcher has already been informed of that,
				// so its refcount has already been decremented.
				continue
			}
			all.decRef(entry, 1)
		}
		e = prev
	}
}

// entityId holds the mongo identifier of an entity.
type entityId struct {
	collection string
	id         interface{}
}

// entityEntry holds an entry in the linked list of all entities known
// to a StateWatcher.
type entityEntry struct {
	// The revno holds the local idea of the latest change.  It is
	// not the same as the transaction revno - this means we can
	// unconditionally move a newly fetched entity to the front of
	// the list without worrying if the revno has changed since the
	// watcher reported it.
	revno int64

	// creationRevno holds the revision number when the
	// entity was created.
	creationRevno int64

	// removed marks whether the entity has been removed.
	// The entry will be deleted when its ref count drops to zero.
	removed bool

	// refCount holds a count of the number of watchers that
	// have seen this entity.
	refCount int

	// info holds the actual information on the entity.
	info EntityInfo
}

// allInfo holds a list of all entities known
// to a StateWatcher.
type allInfo struct {
	st *State
	// newInfo describes how to create a new entity info value given
	// the name of the collection it's stored in.
	newInfo     map[string]func() EntityInfo
	latestRevno int64
	entities    map[entityId]*list.Element
	list        *list.List
}

// newAllInfo returns an allInfo instance holding information about the
// current state of all entities in the environment.
func newAllInfo(st *State) (*allInfo, error) {
	all := &allInfo{
		st:       st,
		entities: make(map[entityId]*list.Element),
		newInfo: map[string]func() EntityInfo{
			st.machines.Name:  func() EntityInfo { return new(MachineInfo) },
			st.units.Name:     func() EntityInfo { return new(UnitInfo) },
			st.services.Name:  func() EntityInfo { return new(ServiceInfo) },
			st.relations.Name: func() EntityInfo { return new(RelationInfo) },
		},
		list: list.New(),
	}
	if err := all.getAll(); err != nil {
		return nil, err
	}
	return all, nil
}

// add adds a new entity to the list.
func (a *allInfo) add(info EntityInfo) {
	a.latestRevno++
	entry := &entityEntry{
		info:  info,
		revno: a.latestRevno,
	}
	a.entities[infoEntityId(a.st, info)] = a.list.PushFront(entry)
}

// decRef decrements the reference count of an entry within the list by
// the given count, removing it if drops to zero.
func (a *allInfo) decRef(entry *entityEntry, n int) {
	if entry.refCount -= n; entry.refCount > 0 {
		return
	}
	if entry.refCount < 0 {
		panic("negative reference count")
	}
	id := infoEntityId(a.st, entry.info)
	elem := a.entities[id]
	if elem == nil {
		panic("delete of non-existent entry")
	}
	if !elem.Value.(*entityEntry).removed {
		panic("deleting entry that has not been marked as removed")
	}
	delete(a.entities, id)
	a.list.Remove(elem)
}

// update updates information on the entity with the given id by
// retrieving its information from mongo.
func (a *allInfo) update(id entityId) error {
	info := a.newInfo[id.collection]()
	collection := collectionForInfo(a.st, info)
	// TODO(rog) investigate ways that this can be made more efficient.
	if err := collection.FindId(info.EntityId()).One(info); err != nil {
		if IsNotFound(err) {
			// The document has been removed since the change notification arrived.
			if elem := a.entities[id]; elem != nil {
				elem.Value.(*entityEntry).removed = true
			}
			return nil
		}
		return fmt.Errorf("cannot get %v from %s: %v", id.id, collection.Name, err)
	}
	if elem := a.entities[id]; elem != nil {
		entry := elem.Value.(*entityEntry)
		// Nothing has changed, so change nothing.
		// TODO(rog) do the comparison more efficiently.
		if reflect.DeepEqual(info, entry.info) {
			return nil
		}
		// We already know about the entity; update its doc.
		a.latestRevno++
		entry.revno = a.latestRevno
		entry.info = info
		a.list.MoveToFront(elem)
	} else {
		a.add(info)
	}
	return nil
}

// getAllCollection fetches all the items in the given collection
// into the given slice.
func (a *allInfo) getAllCollection(c *mgo.Collection, into interface{}) error {
	err := c.Find(nil).All(into)
	if err != nil {
		return fmt.Errorf("cannot get all %s: %v", c.Name, err)
	}
	infos := reflect.ValueOf(into).Elem()
	for i := 0; i < infos.Len(); i++ {
		info := infos.Index(i).Addr().Interface().(EntityInfo)
		a.add(info)
	}
	return nil
}

// getAll retrieves information about all known
// entities from mongo.
func (a *allInfo) getAll() error {
	// TODO(rog) fetch collections concurrently?
	if err := a.getAllCollection(a.st.machines, new([]MachineInfo)); err != nil {
		return err
	}
	if err := a.getAllCollection(a.st.relations, new([]RelationInfo)); err != nil {
		return err
	}
	if err := a.getAllCollection(a.st.units, new([]UnitInfo)); err != nil {
		return err
	}
	if err := a.getAllCollection(a.st.services, new([]ServiceInfo)); err != nil {
		return err
	}
	return nil
}

// The entity kinds are in parent-child order.
var entityKinds = []string{
	"service",
	"relation",
	"machine",
	"unit",
}

// Delta holds details of a change to the environment.
type Delta struct {
	// If Remove is true, the entity has been removed;
	// otherwise it has been created or changed.
	Remove bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
}

// changesSince returns any changes that have occurred since
// the given revno.
func (a *allInfo) changesSince(revno int64) []Delta {
	// Extract all deltas into categorised slices, then build up an
	// overall slice that sends creates before deletes, and orders
	// parents before children on creation, and children before
	// parents on deletion (see kindOrder above).
	e := a.list.Front()
	for ; e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
	}
	if e != nil {
		// We've found an element that we've already seen.
		e = e.Prev()
	} else {
		// We haven't seen any elements, so we want all of them.
		e = a.list.Back()
	}
	if e == nil {
		// Common case: nothing new to see - let's be efficient.
		return nil
	}
	// map from isRemoved to kind to list of deltas.
	deltas := map[bool]map[string][]Delta{
		false: make(map[string][]Delta), // Changed/new entries.
		true:  make(map[string][]Delta), // Removed entries.
	}
	n := 0
	// Iterate from oldest to newest, stopping at the first entry
	// we've already seen.
	for ; e != nil; e = e.Prev() {
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
		m := deltas[entry.removed]
		kind := entry.info.EntityKind()
		m[kind] = append(m[kind], Delta{
			Remove: entry.removed,
			Entity: entry.info,
		})
		n++
	}
	changes := make([]Delta, 0, n)
	// Changes in parent-to-child order
	for _, kind := range entityKinds {
		changes = append(changes, deltas[false][kind]...)
	}
	// Removals in child-to-parent order.
	for i := len(entityKinds) - 1; i >= 0; i-- {
		kind := entityKinds[i]
		changes = append(changes, deltas[true][kind]...)
	}
	return changes
}

// infoEntityId returns the entity id of the given entity document.
func infoEntityId(st *State, info EntityInfo) entityId {
	return entityId{
		collection: collectionForInfo(st, info).Name,
		id:         info.EntityId(),
	}
}

// collectionForInfo returns the collection that holds the
// given kind of entity info. This isn't defined on
// EntityInfo because we don't want to require all
// entities to hold a pointer to the state.
func collectionForInfo(st *State, i EntityInfo) *mgo.Collection {
	switch i.(type) {
	case *MachineInfo:
		return st.machines
	case *RelationInfo:
		return st.relations
	case *ServiceInfo:
		return st.services
	case *UnitInfo:
		return st.units
	}
	panic(fmt.Errorf("unknown entity type %T", i))
}

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityId returns the collection-specific identifier for the entity.
	EntityId() interface{}
	// EntityKind returns the kind of entity (for example "machine", "service", ...)
	EntityKind() string
}

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ServiceInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
)

// MachineInfo holds the information about a Machine
// that is watched by StateWatcher.
type MachineInfo struct {
	Id         string `bson:"_id"`
	InstanceId string
}

func (i *MachineInfo) EntityId() interface{} { return i.Id }
func (i *MachineInfo) EntityKind() string    { return "machine" }

type ServiceInfo struct {
	Name    string `bson:"_id"`
	Exposed bool
}

func (i *ServiceInfo) EntityId() interface{} { return i.Name }
func (i *ServiceInfo) EntityKind() string    { return "service" }

type UnitInfo struct {
	Name    string `bson:"_id"`
	Service string
}

func (i *UnitInfo) EntityId() interface{} { return i.Name }
func (i *UnitInfo) EntityKind() string    { return "service" }

type RelationInfo struct {
	Key string `bson:"_id"`
}

func (i *RelationInfo) EntityId() interface{} { return i.Key }
func (i *RelationInfo) EntityKind() string    { return "service" }
