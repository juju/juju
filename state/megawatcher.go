// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"labix.org/v2/mgo"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/multiwatcher"
	"launchpad.net/juju-core/state/watcher"
)

// allWatcherStateBacking implements allWatcherBacking by
// fetching entities from the State.
type allWatcherStateBacking struct {
	st *State
	// collections
	collectionByName map[string]allWatcherStateCollection
}

type backingMachine machineDoc

func (m *backingMachine) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	info := &params.MachineInfo{
		Id:                       m.Id,
		Life:                     params.Life(m.Life.String()),
		Series:                   m.Series,
		Jobs:                     paramsJobsFromJobs(m.Jobs),
		Addresses:                mergedAddresses(m.MachineAddresses, m.Addresses),
		SupportedContainers:      m.SupportedContainers,
		SupportedContainersKnown: m.SupportedContainersKnown,
	}

	oldInfo := store.Get(info.EntityId())
	if oldInfo == nil {
		// We're adding the entry for the first time,
		// so fetch the associated machine status.
		sdoc, err := getStatus(st, machineGlobalKey(m.Id))
		if err != nil {
			return err
		}
		info.Status = sdoc.Status
		info.StatusInfo = sdoc.StatusInfo
	} else {
		// The entry already exists, so preserve the current status and
		// instance data.
		oldInfo := oldInfo.(*params.MachineInfo)
		info.Status = oldInfo.Status
		info.StatusInfo = oldInfo.StatusInfo
		info.InstanceId = oldInfo.InstanceId
		info.HardwareCharacteristics = oldInfo.HardwareCharacteristics
	}
	// If the machine is been provisioned, fetch the instance id as required,
	// and set instance id and hardware characteristics.
	if m.Nonce != "" && info.InstanceId == "" {
		instanceData, err := getInstanceData(st, m.Id)
		if err == nil {
			info.InstanceId = string(instanceData.InstanceId)
			info.HardwareCharacteristics = hardwareCharacteristics(instanceData)
		} else if !errors.IsNotFound(err) {
			return err
		}
	}
	store.Update(info)
	return nil
}

func (svc *backingMachine) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	store.Remove(params.EntityId{
		Kind: "machine",
		Id:   id,
	})
	return nil
}

func (m *backingMachine) mongoId() interface{} {
	return m.Id
}

type backingUnit unitDoc

func (u *backingUnit) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	info := &params.UnitInfo{
		Name:      u.Name,
		Service:   u.Service,
		Series:    u.Series,
		MachineId: u.MachineId,
		Ports:     u.Ports,
	}
	if u.CharmURL != nil {
		info.CharmURL = u.CharmURL.String()
	}
	oldInfo := store.Get(info.EntityId())
	if oldInfo == nil {
		// We're adding the entry for the first time,
		// so fetch the associated unit status.
		sdoc, err := getStatus(st, unitGlobalKey(u.Name))
		if err != nil {
			return err
		}
		info.Status = sdoc.Status
		info.StatusInfo = sdoc.StatusInfo
	} else {
		// The entry already exists, so preserve the current status.
		oldInfo := oldInfo.(*params.UnitInfo)
		info.Status = oldInfo.Status
		info.StatusInfo = oldInfo.StatusInfo
	}
	publicAddress, privateAddress, err := getUnitAddresses(st, u.Name)
	if err != nil {
		return err
	}
	info.PublicAddress = publicAddress
	info.PrivateAddress = privateAddress
	store.Update(info)
	return nil
}

// getUnitAddresses returns the public and private addresses on a given unit.
// As of 1.18, the addresses are stored on the assigned machine but we retain
// this approach for backwards compatibility.
func getUnitAddresses(st *State, unitName string) (publicAddress, privateAddress string, err error) {
	u, err := st.Unit(unitName)
	if err != nil {
		return "", "", err
	}
	publicAddress, _ = u.PublicAddress()
	privateAddress, _ = u.PrivateAddress()
	return publicAddress, privateAddress, nil
}

func (svc *backingUnit) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	store.Remove(params.EntityId{
		Kind: "unit",
		Id:   id,
	})
	return nil
}

func (m *backingUnit) mongoId() interface{} {
	return m.Name
}

type backingService serviceDoc

func (svc *backingService) updated(st *State, store *multiwatcher.Store, id interface{}) error {

	info := &params.ServiceInfo{
		Name:     svc.Name,
		Exposed:  svc.Exposed,
		CharmURL: svc.CharmURL.String(),
		OwnerTag: svc.fixOwnerTag(),
		Life:     params.Life(svc.Life.String()),
		MinUnits: svc.MinUnits,
	}
	oldInfo := store.Get(info.EntityId())
	needConfig := false
	if oldInfo == nil {
		// We're adding the entry for the first time,
		// so fetch the associated child documents.
		c, err := readConstraints(st, serviceGlobalKey(svc.Name))
		if err != nil {
			return err
		}
		info.Constraints = c
		needConfig = true
	} else {
		// The entry already exists, so preserve the current status.
		oldInfo := oldInfo.(*params.ServiceInfo)
		info.Constraints = oldInfo.Constraints
		if info.CharmURL == oldInfo.CharmURL {
			// The charm URL remains the same - we can continue to
			// use the same config settings.
			info.Config = oldInfo.Config
		} else {
			// The charm URL has changed - we need to fetch the
			// settings from the new charm's settings doc.
			needConfig = true
		}
	}
	if needConfig {
		var err error
		info.Config, _, err = readSettingsDoc(st, serviceSettingsKey(svc.Name, svc.CharmURL))
		if err != nil {
			return err
		}
	}
	store.Update(info)
	return nil
}

func (svc *backingService) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	store.Remove(params.EntityId{
		Kind: "service",
		Id:   id,
	})
	return nil
}

// SCHEMACHANGE
// TODO(mattyw) remove when schema upgrades are possible
func (svc *backingService) fixOwnerTag() string {
	if svc.OwnerTag != "" {
		return svc.OwnerTag
	}
	return "user-admin"
}

func (m *backingService) mongoId() interface{} {
	return m.Name
}

type backingRelation relationDoc

func (r *backingRelation) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	eps := make([]params.Endpoint, len(r.Endpoints))
	for i, ep := range r.Endpoints {
		eps[i] = params.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		}
	}
	info := &params.RelationInfo{
		Key:       r.Key,
		Id:        r.Id,
		Endpoints: eps,
	}
	store.Update(info)
	return nil
}

func (svc *backingRelation) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	store.Remove(params.EntityId{
		Kind: "relation",
		Id:   id,
	})
	return nil
}

func (m *backingRelation) mongoId() interface{} {
	return m.Key
}

type backingAnnotation annotatorDoc

func (a *backingAnnotation) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	info := &params.AnnotationInfo{
		Tag:         a.Tag,
		Annotations: a.Annotations,
	}
	store.Update(info)
	return nil
}

func (svc *backingAnnotation) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	tag, ok := tagForGlobalKey(id.(string))
	if !ok {
		panic(fmt.Errorf("unknown global key %q in state", id))
	}
	store.Remove(params.EntityId{
		Kind: "annotation",
		Id:   tag,
	})
	return nil
}

func (a *backingAnnotation) mongoId() interface{} {
	return a.GlobalKey
}

type backingStatus statusDoc

func (s *backingStatus) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	parentId, ok := backingEntityIdForGlobalKey(id.(string))
	if !ok {
		return nil
	}
	info0 := store.Get(parentId)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *params.UnitInfo:
		newInfo := *info
		newInfo.Status = s.Status
		newInfo.StatusInfo = s.StatusInfo
		newInfo.StatusData = s.StatusData
		info0 = &newInfo
	case *params.MachineInfo:
		newInfo := *info
		newInfo.Status = s.Status
		newInfo.StatusInfo = s.StatusInfo
		newInfo.StatusData = s.StatusData
		info0 = &newInfo
	default:
		panic(fmt.Errorf("status for unexpected entity with id %q; type %T", id, info))
	}
	store.Update(info0)
	return nil
}

func (s *backingStatus) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	// If the status is removed, the parent will follow not long after,
	// so do nothing.
	return nil
}

func (a *backingStatus) mongoId() interface{} {
	panic("cannot find mongo id from status document")
}

type backingConstraints constraintsDoc

func (s *backingConstraints) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	parentId, ok := backingEntityIdForGlobalKey(id.(string))
	if !ok {
		return nil
	}
	info0 := store.Get(parentId)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *params.UnitInfo, *params.MachineInfo:
		// We don't (yet) publish unit or machine constraints.
		return nil
	case *params.ServiceInfo:
		newInfo := *info
		newInfo.Constraints = constraintsDoc(*s).value()
		info0 = &newInfo
	default:
		panic(fmt.Errorf("status for unexpected entity with id %q; type %T", id, info))
	}
	store.Update(info0)
	return nil
}

func (s *backingConstraints) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	return nil
}

func (a *backingConstraints) mongoId() interface{} {
	panic("cannot find mongo id from constraints document")
}

type backingSettings map[string]interface{}

func (s *backingSettings) updated(st *State, store *multiwatcher.Store, id interface{}) error {
	parentId, url, ok := backingEntityIdForSettingsKey(id.(string))
	if !ok {
		return nil
	}
	info0 := store.Get(parentId)
	switch info := info0.(type) {
	case nil:
		// The parent info doesn't exist. Ignore the status until it does.
		return nil
	case *params.ServiceInfo:
		// If we're seeing settings for the service with a different
		// charm URL, we ignore them - we will fetch
		// them again when the service charm changes.
		// By doing this we make sure that the settings in the
		// ServiceInfo are always consistent with the charm URL.
		if info.CharmURL != url {
			break
		}
		newInfo := *info
		cleanSettingsMap(*s)
		newInfo.Config = *s
		info0 = &newInfo
	default:
		return nil
	}
	store.Update(info0)
	return nil
}

func (s *backingSettings) removed(st *State, store *multiwatcher.Store, id interface{}) error {
	return nil
}

func (a *backingSettings) mongoId() interface{} {
	panic("cannot find mongo id from settings document")
}

// backingEntityIdForSettingsKey returns the entity id for the given
// settings key. Any extra information in the key is returned in
// extra.
func backingEntityIdForSettingsKey(key string) (eid params.EntityId, extra string, ok bool) {
	if !strings.HasPrefix(key, "s#") {
		eid, ok = backingEntityIdForGlobalKey(key)
		return
	}
	key = key[2:]
	i := strings.Index(key, "#")
	if i == -1 {
		return params.EntityId{}, "", false
	}
	eid = (&params.ServiceInfo{Name: key[0:i]}).EntityId()
	extra = key[i+1:]
	ok = true
	return
}

// backingEntityIdForGlobalKey returns the entity id for the given global key.
// It returns false if the key is not recognized.
func backingEntityIdForGlobalKey(key string) (params.EntityId, bool) {
	if len(key) < 3 || key[1] != '#' {
		return params.EntityId{}, false
	}
	id := key[2:]
	switch key[0] {
	case 'm':
		return (&params.MachineInfo{Id: id}).EntityId(), true
	case 'u':
		return (&params.UnitInfo{Name: id}).EntityId(), true
	case 's':
		return (&params.ServiceInfo{Name: id}).EntityId(), true
	}
	return params.EntityId{}, false
}

// backingEntityDoc is implemented by the documents in
// collections that the allWatcherStateBacking watches.
type backingEntityDoc interface {
	// updated is called when the document has changed.
	// The mongo _id value of the document is provided in id.
	updated(st *State, store *multiwatcher.Store, id interface{}) error

	// removed is called when the document has changed.
	// The receiving instance will not contain any data.
	// The mongo _id value of the document is provided in id.
	removed(st *State, store *multiwatcher.Store, id interface{}) error

	// mongoId returns the mongo _id field of the document.
	// It is currently never called for subsidiary documents.
	mongoId() interface{}
}

var (
	_ backingEntityDoc = (*backingMachine)(nil)
	_ backingEntityDoc = (*backingUnit)(nil)
	_ backingEntityDoc = (*backingService)(nil)
	_ backingEntityDoc = (*backingRelation)(nil)
	_ backingEntityDoc = (*backingAnnotation)(nil)
	_ backingEntityDoc = (*backingStatus)(nil)
	_ backingEntityDoc = (*backingConstraints)(nil)
	_ backingEntityDoc = (*backingSettings)(nil)
)

// allWatcherStateCollection holds information about a
// collection watched by an allWatcher and the
// type of value we use to store entity information
// for that collection.
type allWatcherStateCollection struct {
	*mgo.Collection

	// infoType stores the type of the info type
	// that we use for this collection.
	infoType reflect.Type
	// subsidiary is true if the collection is used only
	// to modify a primary entity.
	subsidiary bool
}

func newAllWatcherStateBacking(st *State) multiwatcher.Backing {
	collectionByType := make(map[reflect.Type]allWatcherStateCollection)
	b := &allWatcherStateBacking{
		st:               st,
		collectionByName: make(map[string]allWatcherStateCollection),
	}
	collections := []allWatcherStateCollection{{
		Collection: st.machines,
		infoType:   reflect.TypeOf(backingMachine{}),
	}, {
		Collection: st.units,
		infoType:   reflect.TypeOf(backingUnit{}),
	}, {
		Collection: st.services,
		infoType:   reflect.TypeOf(backingService{}),
	}, {
		Collection: st.relations,
		infoType:   reflect.TypeOf(backingRelation{}),
	}, {
		Collection: st.annotations,
		infoType:   reflect.TypeOf(backingAnnotation{}),
	}, {
		Collection: st.statuses,
		infoType:   reflect.TypeOf(backingStatus{}),
		subsidiary: true,
	}, {
		Collection: st.constraints,
		infoType:   reflect.TypeOf(backingConstraints{}),
		subsidiary: true,
	}, {
		Collection: st.settings,
		infoType:   reflect.TypeOf(backingSettings{}),
		subsidiary: true,
	}}
	// Populate the collection maps from the above set of collections.
	for _, c := range collections {
		docType := c.infoType
		if _, ok := collectionByType[docType]; ok {
			panic(fmt.Errorf("duplicate collection type %s", docType))
		}
		collectionByType[docType] = c
		if _, ok := b.collectionByName[c.Name]; ok {
			panic(fmt.Errorf("duplicate collection name %q", c.Name))
		}
		b.collectionByName[c.Name] = c
	}
	return b
}

// Watch watches all the collections.
func (b *allWatcherStateBacking) Watch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.WatchCollection(c.Name, in)
	}
}

// Unwatch unwatches all the collections.
func (b *allWatcherStateBacking) Unwatch(in chan<- watcher.Change) {
	for _, c := range b.collectionByName {
		b.st.watcher.UnwatchCollection(c.Name, in)
	}
}

// GetAll fetches all items that we want to watch from the state.
func (b *allWatcherStateBacking) GetAll(all *multiwatcher.Store) error {
	// TODO(rog) fetch collections concurrently?
	for _, c := range b.collectionByName {
		if c.subsidiary {
			continue
		}
		infoSlicePtr := reflect.New(reflect.SliceOf(c.infoType))
		if err := c.Find(nil).All(infoSlicePtr.Interface()); err != nil {
			return fmt.Errorf("cannot get all %s: %v", c.Name, err)
		}
		infos := infoSlicePtr.Elem()
		for i := 0; i < infos.Len(); i++ {
			info := infos.Index(i).Addr().Interface().(backingEntityDoc)
			info.updated(b.st, all, info.mongoId())
		}
	}
	return nil
}

// Changed updates the allWatcher's idea of the current state
// in response to the given change.
func (b *allWatcherStateBacking) Changed(all *multiwatcher.Store, change watcher.Change) error {
	c, ok := b.collectionByName[change.C]
	if !ok {
		panic(fmt.Errorf("unknown collection %q in fetch request", change.C))
	}
	doc := reflect.New(c.infoType).Interface().(backingEntityDoc)
	// TODO(rog) investigate ways that this can be made more efficient
	// than simply fetching each entity in turn.
	// TODO(rog) avoid fetching documents that we have no interest
	// in, such as settings changes to entities we don't care about.
	err := c.FindId(change.Id).One(doc)
	if err == mgo.ErrNotFound {
		return doc.removed(b.st, all, change.Id)
	}
	if err != nil {
		return err
	}
	return doc.updated(b.st, all, change.Id)
}
