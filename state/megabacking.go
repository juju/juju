package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"reflect"
)

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
			all.add(b.idForInfo(info), info)
		}
	}
	return nil
}

// entityId holds the mongo identifier of an entity.
type entityId struct {
	collection string
	id         interface{}
}

// changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherStateBacking) changed(all *allInfo, change watcher.Change) error {
	id := entityId{
		collection: change.C,
		id:         change.Id,
	}
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

type infoId interface{}

// idForInfo returns the info id of the given entity document.
func (b *allWatcherStateBacking) idForInfo(info params.EntityInfo) infoId {
	c, ok := b.collectionByKind[info.EntityKind()]
	if !ok {
		panic(fmt.Errorf("entity with unknown kind %q", info.EntityKind()))
	}
	return entityId{
		collection: c.Name,
		id:         info.EntityId(),
	}
}
