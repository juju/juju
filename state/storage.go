// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/feature"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
)

// StorageInstance represents the state of a unit or service-wide storage
// instance in the environment.
type StorageInstance interface {
	Entity

	// StorageTag returns the tag for the storage instance.
	StorageTag() names.StorageTag

	// Kind returns the storage instance kind.
	Kind() StorageKind

	// Owner returns the tag of the service or unit that owns this storage
	// instance.
	Owner() names.Tag

	// StorageName returns the name of the storage, as defined in the charm
	// storage metadata. This does not uniquely identify storage instances,
	// but identifies the group that the instances belong to.
	StorageName() string

	// Info returns the storage instance's StorageInstanceInfo, or a
	// NotProvisioned error if the storage instance has not yet been
	// provisioned.
	Info() (StorageInstanceInfo, error)
}

// StorageAttachment represents the state of a unit's attachment to a storage
// instance. A non-shared storage instance will have a single attachment for
// the storage instance's owning unit, whereas a shared storage instance will
// have an attachment for each unit of the service owning the storage instance.
type StorageAttachment interface {
	// StorageInstance returns the tag of the corresponding storage
	// instance.
	StorageInstance() names.StorageTag

	// Unit returns the tag of the corresponding unit.
	Unit() names.UnitTag

	// Info returns the storage attachments's StorageAttachmentInfo, or
	// a NotProvisioned error if the storage attachment has not yet been
	// made.
	Info() (StorageAttachmentInfo, error)
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
	return s.StorageTag()
}

func (s *storageInstance) StorageTag() names.StorageTag {
	return names.NewStorageTag(s.doc.Id)
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

// storageInstanceDoc describes a charm storage instance.
type storageInstanceDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	Id           string      `bson:"id"`
	Kind         StorageKind `bson:"storagekind"`
	Life         Life        `bson:"life"`
	Owner        string      `bson:"owner"`
	StorageName  string      `bson:"storagename"`
	BlockDevices []string    `bson:"blockdevices,omitempty"`

	Info *StorageInstanceInfo `bson:"info,omitempty"`
}

// StorageInstanceInfo records information about the storage instance,
// such as the provisioned size.
type StorageInstanceInfo struct {
	Size uint64 `bson:"size"`
}

type storageAttachment struct {
	doc storageAttachmentDoc
}

func (s *storageAttachment) StorageInstance() names.StorageTag {
	return names.NewStorageTag(s.doc.StorageInstance)
}

func (s *storageAttachment) Unit() names.UnitTag {
	return names.NewUnitTag(s.doc.Unit)
}

func (s *storageAttachment) Info() (StorageAttachmentInfo, error) {
	if s.doc.Info == nil {
		return StorageAttachmentInfo{}, errors.NotProvisionedf(
			"storage %q on unit %q", s.doc.StorageInstance, s.doc.Unit,
		)
	}
	return *s.doc.Info, nil
}

// storageAttachmentDoc describes a unit's attachment to a charm storage
// instance.
type storageAttachmentDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	Unit            string `bson:"unitid"`
	StorageInstance string `bson:"storageinstanceid"`
	Life            Life   `bson:"life"`

	Info *StorageAttachmentInfo `bson:"info"`
}

// StorageAttachmentInfo describes unit-specific information about the
// storage attachment, such as the location where a filesystem is mounted
// path to a block device.
type StorageAttachmentInfo struct {
	// Location is the location of the storage instance,
	// e.g. the mount point.
	Location string `bson:"location"`

	// ReadOnly reports whether the attachment is read-only.
	ReadOnly bool `bson:"read-only"`
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

func storageAttachmentId(unit string, storageInstanceId string) string {
	return fmt.Sprintf("%s#%s", unitGlobalKey(unit), storageInstanceId)
}

// StorageInstance returns the StorageInstance with the specified tag.
func (st *State) StorageInstance(tag names.StorageTag) (StorageInstance, error) {
	storageInstances, cleanup := st.getCollection(storageInstancesC)
	defer cleanup()

	s := storageInstance{st: st}
	err := storageInstances.FindId(tag.Id()).One(&s.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("storage instance %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get storage instance details")
	}
	return &s, nil
}

// RemoveStorageInstance removes the storage instance with the specified tag.
func (st *State) RemoveStorageInstance(tag names.StorageTag) error {
	// TODO(axw) ensure we cannot remove storage instance while
	// there are attachments outstanding.
	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     tag.Id(),
		Remove: true,
	}}
	return st.runTransaction(ops)
}

// createStorageInstanceOps returns txn.Ops for creating storage instances.
//
// The owner tag identifies the entity that owns the storage instance:
// either a unit or a service. Shared storage instances are owned by a
// service, and non-shared storage instances are owned by a unit.
//
// The charm metadata corresponds to the charm that the owner (service/unit)
// is or will be running, and is used to extract storage constraints,
// default values, etc.
//
// The supplied storage constraints are constraints for the storage
// instances to be created, keyed on the storage name. These constraints
// will be correlated with the charm storage metadata for validation
// and supplementing.
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

	includeShared := false
	switch ownerTag.(type) {
	case names.ServiceTag:
		includeShared = true
	case names.UnitTag:
	default:
		return nil, nil, errors.Errorf("expected service or unit tag, got %T", ownerTag)
	}

	templates := make([]template, 0, len(cons))
	for store, cons := range cons {
		charmStorage, ok := charmMeta.Storage[store]
		if !ok {
			return nil, nil, errors.NotFoundf("charm storage %q", store)
		}
		if !includeShared && charmStorage.Shared {
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
				},
			})
			storageInstanceIds = append(storageInstanceIds, id)
		}
	}
	return ops, storageInstanceIds, nil
}

// createStorageAttachmentOps returns txn.Ops for creating storage attachments.
func createStorageAttachmentOps(unit names.UnitTag, storageInstanceIds []string) []txn.Op {
	ops := make([]txn.Op, len(storageInstanceIds))
	for i, storageInstanceId := range storageInstanceIds {
		ops[i] = txn.Op{
			C:      storageAttachmentsC,
			Id:     storageAttachmentId(unit.Id(), storageInstanceId),
			Assert: txn.DocMissing,
			Insert: &storageAttachmentDoc{
				Unit:            unit.Id(),
				StorageInstance: storageInstanceId,
			},
		}
	}
	return ops
}

// StorageAttachments returns the StorageAttachments for the specified unit.
func (st *State) StorageAttachments(unit names.UnitTag) ([]StorageAttachment, error) {
	coll, closer := st.getCollection(storageAttachmentsC)
	defer closer()

	var docs []storageAttachmentDoc
	if err := coll.Find(bson.D{{"unitid", unit.Id()}}).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "cannot get storage attachments for %s", unit.Id())
	}
	storageAttachments := make([]StorageAttachment, len(docs))
	for i, doc := range docs {
		storageAttachments[i] = &storageAttachment{doc}
	}
	return storageAttachments, nil
}

// SetStorageAttachmentInfo sets the storage attachment information for the
// storage attachment relating to the specified storage instance and unit.
func (st *State) SetStorageAttachmentInfo(
	storage names.StorageTag, unit names.UnitTag, info StorageAttachmentInfo,
) error {
	ops := []txn.Op{{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(unit.Id(), storage.Id()),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"info", &info}}}},
	}}
	return st.runTransaction(ops)
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

func validateStorageConstraints(st *State, allCons map[string]StorageConstraints, charmMeta *charm.Meta) error {
	// TODO(axw) stop checking feature flag once storage has graduated.
	if !featureflag.Enabled(feature.Storage) {
		return nil
	}
	for name, cons := range allCons {
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
		if charmStorage.CountMin < 0 {
			return errors.Errorf(
				"charm %q store %q: min count %v must be greater than 0",
				charmMeta.Name, name,
				charmStorage.CountMin,
			)
		}
		if charmStorage.CountMin == 0 {
			charmStorage.CountMin = 1
		}
		kind := storage.StorageKindUnknown
		switch charmStorage.Type {
		case charm.StorageBlock:
			kind = storage.StorageKindBlock
		case charm.StorageFilesystem:
			kind = storage.StorageKindFilesystem
		}
		if poolName, err := validateStoragePool(st, cons.Pool, kind); err != nil {
			if err == ErrNoDefaultStoragePool {
				err = errors.Maskf(err, "no storage pool specified and no default available for %q storage", name)
			}
			return err
		} else {
			cons.Pool = poolName
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
		// TODO - use charm min size when available
		if cons.Size == 0 {
			// TODO(axw) this doesn't really belong in a validation
			// method. We should separate setting defaults from
			// validating, the latter of which should be non-modifying.
			cons.Size = 1024
		}
		// Replace in case pool or size were updated.
		allCons[name] = cons
	}
	// Ensure all stores have constraints specified. Defaults should have
	// been set by this point, if the user didn't specify constraints.
	for name := range charmMeta.Storage {
		if _, ok := allCons[name]; !ok {
			return errors.Errorf("no constraints specified for store %q", name)
		}
	}
	return nil
}

// ErrNoDefaultStoragePool is returned when a storage pool is required but none
// is specified nor available as a default.
var ErrNoDefaultStoragePool = fmt.Errorf("no storage pool specifed and no default available")

func validateStoragePool(st *State, poolName string, kind storage.StorageKind) (string, error) {
	conf, err := st.EnvironConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	envType := conf.Type()
	// If no pool specified, use the default if registered.
	if poolName == "" {
		defaultPool, ok := storage.DefaultPool(envType, kind)
		if ok {
			logger.Infof("no storage pool specified, using default pool %q", defaultPool)
			poolName = defaultPool
		} else {
			return "", ErrNoDefaultStoragePool
		}
	}
	// Ensure the pool type is supported by the environment.
	pm := pool.NewPoolManager(NewStateSettings(st))
	p, err := pm.Get(poolName)
	if err != nil {
		return "", errors.Trace(err)
	}
	providerType := p.Type()
	if !storage.IsProviderSupported(envType, providerType) {
		return "", errors.Errorf(
			"pool %q uses storage provider %q which is not supported for environments of type %q",
			poolName,
			providerType,
			envType,
		)
	}
	return poolName, nil
}
