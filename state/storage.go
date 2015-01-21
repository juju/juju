// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// StorageInstance represents the state of a unit or service-wide storage
// instance in the environment.
type StorageInstance interface {
	// Tag returns the tag for the storage instance.
	Tag() names.Tag

	// Id returns the unique ID of the storage instance.
	Id() string

	// Kind returns the storage instance kind.
	Kind() StorageKind

	// Owner returns the tag of the service or unit that owns this storage
	// instance.
	Owner() names.Tag

	// StorageName returns the name of the storage, as defined in the charm
	// storage metadata. This does not uniquely identify storage instances,
	// but identifies the group that the instances belong to.
	StorageName() string

	// BlockDevices returns the names of the block devices assigned to this
	// storage instance.
	BlockDeviceNames() []string

	// Info returns the storage instance's StorageInstanceInfo, or a
	// NotProvisioned error if the storage instance has not yet been
	// provisioned.
	Info() (StorageInstanceInfo, error)

	// Params returns the parameters for provisioning the storage instance,
	// if it has not already been provisioned. Params returns true if the
	// returned parameters are usable for provisioning, otherwise false.
	Params() (StorageInstanceParams, bool)

	// Remove removes the storage instance and any remaining references to
	// it. If the storage instance no longer exists, the call is a no-op.
	Remove() error
}

// StorageKind defines the type of a store: whether it is a block device
// or a filesystem.
type StorageKind int

const (
	StorageKindUnknown StorageKind = iota
	StorageKindBlock
	StorageKindFilesystem
)

type storageInstance struct {
	st  *State
	doc storageInstanceDoc
}

func (s *storageInstance) Tag() names.Tag {
	return names.NewStorageTag(s.doc.Id)
}

func (s *storageInstance) Id() string {
	return s.doc.Id
}

func (s *storageInstance) Kind() StorageKind {
	return s.doc.Kind
}

func (s *storageInstance) Owner() names.Tag {
	tag, err := names.ParseTag(s.doc.Owner)
	if err != nil {
		// This should be impossible; we do not expose
		// a means of modifying the owner tag.
		panic(err)
	}
	return tag
}

func (s *storageInstance) StorageName() string {
	return s.doc.StorageName
}

func (s *storageInstance) Info() (StorageInstanceInfo, error) {
	if s.doc.Info == nil {
		return StorageInstanceInfo{}, errors.NotProvisionedf("storage instance %q", s.doc.Id)
	}
	return *s.doc.Info, nil
}

func (s *storageInstance) Params() (StorageInstanceParams, bool) {
	if s.doc.Params == nil {
		return StorageInstanceParams{}, false
	}
	return *s.doc.Params, true
}

func (s *storageInstance) BlockDeviceNames() []string {
	return s.doc.BlockDevices
}

func (s *storageInstance) Remove() error {
	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     s.doc.Id,
		Remove: true,
	}}
	tag, err := names.ParseTag(s.doc.Owner)
	if err != nil {
		return errors.Trace(err)
	}
	switch tag.(type) {
	case names.UnitTag:
		ops = append(ops, txn.Op{
			C:      unitsC,
			Id:     tag.Id(),
			Update: bson.D{{"$pull", bson.D{{"storageinstances", s.doc.Id}}}},
		})
	}
	for _, blockDevice := range s.doc.BlockDevices {
		ops = append(ops, txn.Op{
			C:      blockDevicesC,
			Id:     blockDevice,
			Assert: bson.D{{"storageinstanceid", s.doc.Id}},
			Update: bson.D{{"$unset", bson.D{{"storageinstanceid", nil}}}},
		})
	}
	return s.st.runTransaction(ops)
}

// storageInstanceDoc describes a charm storage instance.
type storageInstanceDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	Id           string      `bson:"id"`
	Kind         StorageKind `bson:"storagekind"`
	Owner        string      `bson:"owner"`
	StorageName  string      `bson:"storagename"`
	Pool         string      `bson:"pool"`
	BlockDevices []string    `bson:"blockdevices,omitempty"`

	Info   *StorageInstanceInfo   `bson:"info,omitempty"`
	Params *StorageInstanceParams `bson:"params,omitempty"`
}

// StorageInfo describes information about the storage instance.
type StorageInstanceInfo struct {
	// Location is the location of the storage
	// instance, e.g. the mount point.
	Location string `bson:"location"`
}

// StorageInstanceParams records parameters for provisioning a new
// storage instance.
type StorageInstanceParams struct {
	Size     uint64 `bson:"size"`
	Location string `bson:"location,omitempty"`
	ReadOnly bool   `bson:"read-only"`
}

// newStorageInstanceId returns a unique storage instance name. The name
// incorporates the storage name as defined in the charm storage metadata,
// and a unique sequence number.
func newStorageInstanceId(st *State, store string) (string, error) {
	seq, err := st.sequence("stores")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("%s/%v", store, seq), nil
}

// StorageInstance returns the StorageInstance with the specified ID.
func (st *State) StorageInstance(id string) (StorageInstance, error) {
	storageInstances, cleanup := st.getCollection(storageInstancesC)
	defer cleanup()

	s := storageInstance{st: st}
	err := storageInstances.FindId(id).One(&s.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("storage instance %q", id)
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get storage instance details")
	}
	return &s, nil
}

func createStorageInstanceOps(
	st *State,
	ownerTag names.Tag,
	charmMeta *charm.Meta,
	cons map[string]StorageConstraints,
) (ops []txn.Op, storageInstanceIds []string, err error) {

	type template struct {
		storageName string
		meta        charm.Storage
		cons        StorageConstraints
	}

	// Create a StorageInstanceParams for each store (one for each Count
	// in the constraint), ignoring shared stores. We store the params
	// directly on the storage instances.
	templates := make([]template, 0, len(cons))
	for store, cons := range cons {
		charmStorage, ok := charmMeta.Storage[store]
		if !ok {
			return nil, nil, errors.NotFoundf("charm storage %q", store)
		}
		if charmStorage.Shared {
			continue
		}
		templates = append(templates, template{
			storageName: store,
			meta:        charmStorage,
			cons:        cons,
		})
	}

	ops = make([]txn.Op, 0, len(templates))
	storageInstanceIds = make([]string, 0, len(templates))
	for _, t := range templates {
		params := StorageInstanceParams{
			Size:     t.cons.Size,
			Location: t.meta.Location,
			ReadOnly: t.meta.ReadOnly,
		}

		owner := ownerTag.String()
		var kind StorageKind
		switch t.meta.Type {
		case charm.StorageBlock:
			kind = StorageKindBlock
		case charm.StorageFilesystem:
			kind = StorageKindFilesystem
		default:
			return nil, nil, errors.Errorf("unknown storage type %q", t.meta.Type)
		}

		for i := uint64(0); i < t.cons.Count; i++ {
			id, err := newStorageInstanceId(st, t.storageName)
			if err != nil {
				return nil, nil, errors.Annotate(err, "cannot generate storage instance name")
			}
			ops = append(ops, txn.Op{
				C:      storageInstancesC,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: &storageInstanceDoc{
					Id:          id,
					Kind:        kind,
					Owner:       owner,
					StorageName: t.storageName,
					Pool:        t.cons.Pool,
					Params:      &params,
				},
			})
			storageInstanceIds = append(storageInstanceIds, id)
		}
	}
	return ops, storageInstanceIds, nil
}

// removeStorageInstancesOps returns the transaction operations to remove all
// storage instances owned by the specified entity.
func removeStorageInstancesOps(st *State, owner names.Tag) ([]txn.Op, error) {
	coll, closer := st.getCollection(storageInstancesC)
	defer closer()

	var docs []storageInstanceDoc
	err := coll.Find(bson.D{{"owner", owner.String()}}).Select(bson.D{{"id", true}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage instances for %s", owner)
	}
	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      storageInstancesC,
			Id:     doc.Id,
			Remove: true,
		}
	}
	return ops, nil
}

func readStorageInstances(st *State, owner names.Tag) ([]StorageInstance, error) {
	coll, closer := st.getCollection(storageInstancesC)
	defer closer()

	var docs []storageInstanceDoc
	if err := coll.Find(bson.D{{"owner", owner.String()}}).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "cannot get storage instances for %s", owner)
	}
	storageInstances := make([]StorageInstance, len(docs))
	for i, doc := range docs {
		storageInstances[i] = &storageInstance{st, doc}
	}
	return storageInstances, nil
}

// storageConstraintsDoc contains storage constraints for an entity.
type storageConstraintsDoc struct {
	DocID       string                        `bson:"_id"`
	EnvUUID     string                        `bson:"env-uuid"`
	Constraints map[string]StorageConstraints `bson:"constraints"`
}

// StorageConstraints contains the user-specified constraints for provisioning
// storage instances for a service unit.
type StorageConstraints struct {
	// Pool is the name of the storage pool from which to provision the
	// storage instances.
	Pool string `bson:"pool"`

	// Size is the required size of the storage instances, in MiB.
	Size uint64 `bson:"size"`

	// Count is the required number of storage instances.
	Count uint64 `bson:"count"`
}

func createStorageConstraintsOp(key string, cons map[string]StorageConstraints) txn.Op {
	return txn.Op{
		C:      storageConstraintsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &storageConstraintsDoc{
			Constraints: cons,
		},
	}
}

func removeStorageConstraintsOp(key string) txn.Op {
	return txn.Op{
		C:      storageConstraintsC,
		Id:     key,
		Remove: true,
	}
}

func readStorageConstraints(st *State, key string) (map[string]StorageConstraints, error) {
	coll, closer := st.getCollection(storageConstraintsC)
	defer closer()

	var doc storageConstraintsDoc
	err := coll.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage constraints for %q", key)
	}
	return doc.Constraints, nil
}

func validateStorageConstraints(st *State, cons map[string]StorageConstraints, charmMeta *charm.Meta) error {
	// TODO(axw) stop checking feature flag once storage has graduated.
	if !featureflag.Enabled(storage.FeatureFlag) {
		return nil
	}
	for name, cons := range cons {
		charmStorage, ok := charmMeta.Storage[name]
		if !ok {
			return errors.Errorf("charm %q has no store called %q", charmMeta.Name, name)
		}
		if charmStorage.Shared {
			// TODO(axw) implement shared storage support.
			return errors.Errorf(
				"charm %q store %q: shared storage support not implemented",
				charmMeta.Name, name,
			)
		}
		if cons.Pool != "" {
			// TODO(axw) when we support pools, we should invert the test;
			// the caller should carry out the logic for determining the
			// default pool and so on.
			return errors.Errorf("storage pools are not implemented")
		}
		if cons.Count < uint64(charmStorage.CountMin) {
			return errors.Errorf(
				"charm %q store %q: %d instances required, %d specified",
				charmMeta.Name, name, charmStorage.CountMin, cons.Count,
			)
		}
		if charmStorage.CountMax >= 0 && cons.Count > uint64(charmStorage.CountMax) {
			return errors.Errorf(
				"charm %q store %q: at most %d instances supported, %d specified",
				charmMeta.Name, name, charmStorage.CountMax, cons.Count,
			)
		}
	}
	// Ensure all stores have constraints specified. Defaults should have
	// been set by this point, if the user didn't specify constraints.
	for name := range charmMeta.Storage {
		if _, ok := cons[name]; !ok {
			return errors.Errorf("no constraints specified for store %q", name)
		}
	}
	return nil
}
