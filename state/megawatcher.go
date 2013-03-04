package state

import (
	"container/list"
	"fmt"
	"labix.org/v2/mgo"
	"reflect"
	"launchpad.net/juju-core/log"
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

// update updates information on the entity with the given id by
// retrieving its information from mongo.
func (a *allInfo) update(id entityId) error {
	info := a.newInfo[id.collection]()
	collection := collectionForInfo(a.st, info)
	// TODO(rog) investigate ways that this can be made more efficient.
	if err := collection.FindId(id.id).One(info); err != nil {
		if err == mgo.ErrNotFound {
			log.Printf("id %#v not found", id)
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
