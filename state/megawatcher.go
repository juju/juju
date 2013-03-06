package state

import (
	"container/list"
	"reflect"
)

// entityId holds the mongo identifier of an entity.
type entityId struct {
	collection string
	id         interface{}
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

	// removed marks whether the entity has been removed.
	// The entry will be deleted when its ref count drops to zero.
	removed bool

	// info holds the actual information on the entity.
	info EntityInfo
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
func (a *allInfo) add(id entityId, info EntityInfo) {
	if a.entities[id] != nil {
		panic("adding new entry with duplicate id")
	}
	n := a.list.Len()
	a.latestRevno++
	entry := &entityEntry{
		info:  info,
		revno: a.latestRevno,
	}
	a.entities[id] = a.list.PushFront(entry)
	if a.list.Len() != n+1 || len(a.entities) != n+1 {
		panic("huh?!")
	}
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
// been removed from the state.
func (a *allInfo) markRemoved(id entityId) {
	if elem := a.entities[id]; elem != nil {
		entry := elem.Value.(*entityEntry)
		a.latestRevno++
		entry.revno = a.latestRevno
		entry.removed = true
		a.list.MoveToFront(elem)
	}
}

// update updates the information for the entity with
// the given id.
func (a *allInfo) update(id entityId, info EntityInfo) {
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
// the given revno, oldest first.
func (a *allInfo) changesSince(revno int64) []Delta {
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
		changes = append(changes, Delta{
			Remove: entry.removed,
			Entity: entry.info,
		})
	}
	return changes
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
