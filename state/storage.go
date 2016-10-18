// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
)

// StorageInstance represents the state of a unit or application-wide storage
// instance in the model.
type StorageInstance interface {
	Entity

	// StorageTag returns the tag for the storage instance.
	StorageTag() names.StorageTag

	// Kind returns the storage instance kind.
	Kind() StorageKind

	// Owner returns the tag of the application or unit that owns this storage
	// instance.
	Owner() names.Tag

	// StorageName returns the name of the storage, as defined in the charm
	// storage metadata. This does not uniquely identify storage instances,
	// but identifies the group that the instances belong to.
	StorageName() string

	// Life reports whether the storage instance is Alive, Dying or Dead.
	Life() Life
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

	// Life reports whether the storage attachment is Alive, Dying or Dead.
	Life() Life
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

// String returns a human readable string represting the type.
func (k StorageKind) String() string {
	switch k {
	case StorageKindBlock:
		return "block"
	case StorageKindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}

// parseStorageKind is used by the migration code to go from the
// string representation back to the enum.
func parseStorageKind(value string) StorageKind {
	switch value {
	case "block":
		return StorageKindBlock
	case "filesystem":
		return StorageKindFilesystem
	default:
		return StorageKindUnknown
	}
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

func (s *storageInstance) Life() Life {
	return s.doc.Life
}

// entityStorageRefcountKey returns a key for refcounting charm storage
// for a specific entity. Each time a storage instance is created, the
// named store's refcount is incremented; and decremented when removed.
func entityStorageRefcountKey(owner names.Tag, storageName string) string {
	return fmt.Sprintf("storage#%s#%s", owner.String(), storageName)
}

// storageInstanceDoc describes a charm storage instance.
type storageInstanceDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	Id              string      `bson:"id"`
	Kind            StorageKind `bson:"storagekind"`
	Life            Life        `bson:"life"`
	Owner           string      `bson:"owner"`
	StorageName     string      `bson:"storagename"`
	AttachmentCount int         `bson:"attachmentcount"`
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

func (s *storageAttachment) Life() Life {
	return s.doc.Life
}

// storageAttachmentDoc describes a unit's attachment to a charm storage
// instance.
type storageAttachmentDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	Unit            string `bson:"unitid"`
	StorageInstance string `bson:"storageid"`
	Life            Life   `bson:"life"`
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
	s, err := st.storageInstance(tag)
	return s, err
}

func (st *State) storageInstance(tag names.StorageTag) (*storageInstance, error) {
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

// AllStorageInstances lists all storage instances currently in state
// for this Juju model.
func (st *State) AllStorageInstances() (storageInstances []StorageInstance, err error) {
	storageCollection, closer := st.getCollection(storageInstancesC)
	defer closer()

	sdocs := []storageInstanceDoc{}
	err = storageCollection.Find(nil).All(&sdocs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all storage instances")
	}
	for _, doc := range sdocs {
		storageInstances = append(storageInstances, &storageInstance{st, doc})
	}
	return
}

// DestroyStorageInstance ensures that the storage instance and all its
// attachments will be removed at some point; if the storage instance has
// no attachments, it will be removed immediately.
func (st *State) DestroyStorageInstance(tag names.StorageTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy storage %q", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageInstance(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		switch ops, err := st.destroyStorageInstanceOps(s); err {
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, errors.Trace(err)
		}
	}
	return st.run(buildTxn)
}

func (st *State) destroyStorageInstanceOps(s *storageInstance) ([]txn.Op, error) {
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	if s.doc.AttachmentCount == 0 {
		// There are no attachments remaining, so we can
		// remove the storage instance immediately.
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		assert := append(hasNoAttachments, isAliveDoc...)
		return removeStorageInstanceOps(st, s.Owner(), s.StorageTag(), assert)
	}
	// There are still attachments: the storage instance will be removed
	// when the last attachment is removed. We schedule a cleanup to destroy
	// attachments.
	notLastRefs := bson.D{
		{"life", Alive},
		{"attachmentcount", bson.D{{"$gt", 0}}},
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	ops := []txn.Op{
		newCleanupOp(cleanupAttachmentsForDyingStorage, s.doc.Id),
		{
			C:      storageInstancesC,
			Id:     s.doc.Id,
			Assert: notLastRefs,
			Update: update,
		},
	}
	return ops, nil
}

// removeStorageInstanceOps removes the storage instance with the given
// tag from state, if the specified assertions hold true.
func removeStorageInstanceOps(
	st *State,
	owner names.Tag,
	tag names.StorageTag,
	assert bson.D,
) ([]txn.Op, error) {

	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     tag.Id(),
		Assert: assert,
		Remove: true,
	}}

	machineStorageOp := func(c string, id string) txn.Op {
		return txn.Op{
			C:      c,
			Id:     id,
			Assert: bson.D{{"storageid", tag.Id()}},
			Update: bson.D{{"$set", bson.D{{"storageid", ""}}}},
		}
	}

	// If the storage instance has an assigned volume and/or filesystem,
	// unassign them. Any volumes and filesystems bound to the storage
	// will be destroyed.
	volume, err := st.storageInstanceVolume(tag)
	if err == nil {
		ops = append(ops, machineStorageOp(
			volumesC, volume.Tag().Id(),
		))
		if volume.LifeBinding() == tag {
			ops = append(ops, destroyVolumeOps(st, volume)...)
		}
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	filesystem, err := st.storageInstanceFilesystem(tag)
	if err == nil {
		ops = append(ops, machineStorageOp(
			filesystemsC, filesystem.Tag().Id(),
		))
		if filesystem.LifeBinding() == tag {
			ops = append(ops, destroyFilesystemOps(st, filesystem)...)
		}
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	// Decrement the charm storage reference count.
	refcounts, closer := st.getCollection(refcountsC)
	defer closer()
	storageName, err := names.StorageName(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageRefcountKey := entityStorageRefcountKey(owner, storageName)
	decRefOp, _, err := nsRefcounts.DyingDecRefOp(refcounts, storageRefcountKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, decRefOp)

	return ops, nil
}

// machineAssignable is used by createStorageOps to determine what machine
// storage needs to be created. This is implemented by Unit.
type machineAssignable interface {
	machine() (*Machine, error)
	noAssignedMachineOp() txn.Op
}

// createStorageOps returns txn.Ops for creating storage instances
// and attachments for the newly created unit or service.
//
// The entity tag identifies the entity that owns the storage instance
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
//
// maybeMachineAssignable may be nil, or an machineAssignable which
// describes the entity's machine assignment. If the entity is assigned
// to a machine, then machine storage will be created.
func createStorageOps(
	st *State,
	entityTag names.Tag,
	charmMeta *charm.Meta,
	cons map[string]StorageConstraints,
	maybeMachineAssignable machineAssignable,
) (ops []txn.Op, numStorageAttachments int, err error) {

	type template struct {
		storageName string
		meta        charm.Storage
		cons        StorageConstraints
	}

	createdShared := false
	switch entityTag := entityTag.(type) {
	case names.ApplicationTag:
		createdShared = true
	case names.UnitTag:
	default:
		return nil, -1, errors.Errorf("expected application or unit tag, got %T", entityTag)
	}

	// Create storage instances in order of name, to simplify testing.
	storageNames := set.NewStrings()
	for name := range cons {
		storageNames.Add(name)
	}

	templates := make([]template, 0, len(cons))
	for _, store := range storageNames.SortedValues() {
		cons := cons[store]
		charmStorage, ok := charmMeta.Storage[store]
		if !ok {
			return nil, -1, errors.NotFoundf("charm storage %q", store)
		}
		if cons.Count == 0 {
			continue
		}
		if createdShared != charmStorage.Shared {
			// services only get shared storage instances,
			// units only get non-shared storage instances.
			continue
		}
		templates = append(templates, template{
			storageName: store,
			meta:        charmStorage,
			cons:        cons,
		})
	}

	refcounts, closer := st.getCollection(refcountsC)
	defer closer()

	ops = make([]txn.Op, 0, len(templates)*3)
	for _, t := range templates {
		owner := entityTag.String()
		var kind StorageKind
		switch t.meta.Type {
		case charm.StorageBlock:
			kind = StorageKindBlock
		case charm.StorageFilesystem:
			kind = StorageKindFilesystem
		default:
			return nil, -1, errors.Errorf("unknown storage type %q", t.meta.Type)
		}

		// Increment reference counts for the named storage for each
		// instance we create. We'll use the reference counts to ensure
		// we don't exceed limits when adding storage, and for
		// maintaining model integrity during charm upgrades.
		storageRefcountKey := entityStorageRefcountKey(entityTag, t.storageName)
		incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, storageRefcountKey, int(t.cons.Count))
		if err != nil {
			return nil, -1, errors.Trace(err)
		}
		ops = append(ops, incRefOp)

		for i := uint64(0); i < t.cons.Count; i++ {
			id, err := newStorageInstanceId(st, t.storageName)
			if err != nil {
				return nil, -1, errors.Annotate(err, "cannot generate storage instance name")
			}
			doc := &storageInstanceDoc{
				Id:          id,
				Kind:        kind,
				Owner:       owner,
				StorageName: t.storageName,
			}
			var machineOps []txn.Op
			if unitTag, ok := entityTag.(names.UnitTag); ok {
				doc.AttachmentCount = 1
				storage := names.NewStorageTag(id)
				ops = append(ops, createStorageAttachmentOp(storage, unitTag))
				numStorageAttachments++

				if maybeMachineAssignable != nil {
					var err error
					machineOps, err = unitAssignedMachineStorageOps(
						st, unitTag, charmMeta, cons,
						&storageInstance{st, *doc},
						maybeMachineAssignable,
					)
					if err != nil {
						return nil, -1, errors.Annotatef(
							err, "creating machine storage for storage %s", id,
						)
					}
				}
			}
			ops = append(ops, txn.Op{
				C:      storageInstancesC,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: doc,
			})
			ops = append(ops, machineOps...)
		}
	}

	// TODO(axw) create storage attachments for each shared storage
	// instance owned by the service.
	//
	// TODO(axw) prevent creation of shared storage after service
	// creation, because the only sane time to add storage attachments
	// is when units are added to said service.

	return ops, numStorageAttachments, nil
}

// unitAssignedMachineStorageOps returns ops for creating volumes, filesystems
// and their attachments to the machine that the specified unit is assigned to,
// corresponding to the specified storage instance.
//
// If the unit is not assigned to a machine, then ops will be returned to assert
// this, and no error will be returned.
func unitAssignedMachineStorageOps(
	st *State,
	unitTag names.UnitTag,
	charmMeta *charm.Meta,
	cons map[string]StorageConstraints,
	storage StorageInstance,
	machineAssignable machineAssignable,
) (ops []txn.Op, err error) {
	storageParams, err := machineStorageParamsForStorageInstance(
		st, charmMeta, unitTag, cons, storage,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	m, err := machineAssignable.machine()
	if err != nil {
		if errors.IsNotAssigned(err) {
			// The unit is not assigned to a machine; return
			// txn.Op that ensures that this remains the case
			// until the transaction is committed.
			return []txn.Op{machineAssignable.noAssignedMachineOp()}, nil
		}
		return nil, errors.Trace(err)
	}

	if err := validateDynamicMachineStorageParams(m, storageParams); err != nil {
		return nil, errors.Trace(err)
	}
	storageOps, volumeAttachments, filesystemAttachments, err := st.machineStorageOps(
		&m.doc, storageParams,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	attachmentOps, err := addMachineStorageAttachmentsOps(
		m, volumeAttachments, filesystemAttachments,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageOps = append(storageOps, attachmentOps...)
	return storageOps, nil
}

// createStorageAttachmentOps returns a txn.Op for creating a storage attachment.
// The caller is responsible for updating the attachmentcount field of the storage
// instance.
func createStorageAttachmentOp(storage names.StorageTag, unit names.UnitTag) txn.Op {
	return txn.Op{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(unit.Id(), storage.Id()),
		Assert: txn.DocMissing,
		Insert: &storageAttachmentDoc{
			Unit:            unit.Id(),
			StorageInstance: storage.Id(),
		},
	}
}

// StorageAttachments returns the StorageAttachments for the specified storage
// instance.
func (st *State) StorageAttachments(storage names.StorageTag) ([]StorageAttachment, error) {
	query := bson.D{{"storageid", storage.Id()}}
	attachments, err := st.storageAttachments(query)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage attachments for storage %s", storage.Id())
	}
	return attachments, nil
}

// UnitStorageAttachments returns the StorageAttachments for the specified unit.
func (st *State) UnitStorageAttachments(unit names.UnitTag) ([]StorageAttachment, error) {
	query := bson.D{{"unitid", unit.Id()}}
	attachments, err := st.storageAttachments(query)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage attachments for unit %s", unit.Id())
	}
	return attachments, nil
}

func (st *State) storageAttachments(query bson.D) ([]StorageAttachment, error) {
	coll, closer := st.getCollection(storageAttachmentsC)
	defer closer()

	var docs []storageAttachmentDoc
	if err := coll.Find(query).All(&docs); err != nil {
		return nil, err
	}
	storageAttachments := make([]StorageAttachment, len(docs))
	for i, doc := range docs {
		storageAttachments[i] = &storageAttachment{doc}
	}
	return storageAttachments, nil
}

// StorageAttachment returns the StorageAttachment wit hthe specified tags.
func (st *State) StorageAttachment(storage names.StorageTag, unit names.UnitTag) (StorageAttachment, error) {
	att, err := st.storageAttachment(storage, unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return att, nil
}

func (st *State) storageAttachment(storage names.StorageTag, unit names.UnitTag) (*storageAttachment, error) {
	coll, closer := st.getCollection(storageAttachmentsC)
	defer closer()
	var s storageAttachment
	err := coll.FindId(storageAttachmentId(unit.Id(), storage.Id())).One(&s.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("storage attachment %s:%s", storage.Id(), unit.Id())
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage attachment %s:%s", storage.Id(), unit.Id())
	}
	return &s, nil
}

// DestroyStorageAttachment ensures that the existing storage attachments of
// the specified unit are removed at some point.
func (st *State) DestroyUnitStorageAttachments(unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy unit %s storage attachments", unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachments, err := st.UnitStorageAttachments(unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := make([]txn.Op, 0, len(attachments))
		for _, attachment := range attachments {
			if attachment.Life() != Alive {
				continue
			}
			ops = append(ops, destroyStorageAttachmentOps(
				attachment.StorageInstance(), unit,
			)...)
		}
		if len(ops) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		return ops, nil
	}
	return st.run(buildTxn)
}

// DestroyStorageAttachment ensures that the storage attachment will be
// removed at some point.
func (st *State) DestroyStorageAttachment(storage names.StorageTag, unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy storage attachment %s:%s", storage.Id(), unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageAttachment(storage, unit)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if s.doc.Life == Dying {
			return nil, jujutxn.ErrNoOperations
		}
		return destroyStorageAttachmentOps(storage, unit), nil
	}
	return st.run(buildTxn)
}

func destroyStorageAttachmentOps(storage names.StorageTag, unit names.UnitTag) []txn.Op {
	ops := []txn.Op{{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(unit.Id(), storage.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
	return ops
}

// Remove removes the storage attachment from state, and may remove its storage
// instance as well, if the storage instance is Dying and no other references to
// it exist. It will fail if the storage attachment is not Dying.
func (st *State) RemoveStorageAttachment(storage names.StorageTag, unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove storage attachment %s:%s", storage.Id(), unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageAttachment(storage, unit)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		inst, err := st.storageInstance(storage)
		if errors.IsNotFound(err) {
			// This implies that the attachment was removed
			// after the call to st.storageAttachment.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := removeStorageAttachmentOps(st, s, inst)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return st.run(buildTxn)
}

func removeStorageAttachmentOps(
	st *State,
	s *storageAttachment,
	si *storageInstance,
) ([]txn.Op, error) {
	if s.doc.Life != Dying {
		return nil, errors.New("storage attachment is not dying")
	}
	ops := []txn.Op{{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(s.doc.Unit, s.doc.StorageInstance),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, {
		C:      unitsC,
		Id:     s.doc.Unit,
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"storageattachmentcount", -1}}}},
	}}
	if si.doc.AttachmentCount == 1 {
		var hasLastRef bson.D
		if si.doc.Life == Dying {
			hasLastRef = bson.D{{"life", Dying}, {"attachmentcount", 1}}
		} else if si.doc.Owner == names.NewUnitTag(s.doc.Unit).String() {
			hasLastRef = bson.D{{"attachmentcount", 1}}
		}
		if len(hasLastRef) > 0 {
			// Either the storage instance is dying, or its owner
			// is a unit; in either case, no more attachments can
			// be added to the instance, so it can be removed.
			siOps, err := removeStorageInstanceOps(
				st, si.Owner(), si.StorageTag(), hasLastRef,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, siOps...)
			return ops, nil
		}
	}
	decrefOp := txn.Op{
		C:      storageInstancesC,
		Id:     si.doc.Id,
		Update: bson.D{{"$inc", bson.D{{"attachmentcount", -1}}}},
	}
	if si.doc.Life == Alive {
		// This may be the last reference, but the storage instance is
		// still alive. The storage instance will be removed when its
		// Destroy method is called, if it has no attachments.
		decrefOp.Assert = bson.D{
			{"life", Alive},
			{"attachmentcount", bson.D{{"$gt", 0}}},
		}
	} else {
		// If it's not the last reference when we checked, we want to
		// allow for concurrent attachment removals but want to ensure
		// that we don't drop to zero without removing the storage
		// instance.
		decrefOp.Assert = bson.D{
			{"life", Dying},
			{"attachmentcount", bson.D{{"$gt", 1}}},
		}
	}
	ops = append(ops, decrefOp)
	return ops, nil
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
	ops := make([]txn.Op, 0, len(docs))
	for _, doc := range docs {
		tag := names.NewStorageTag(doc.Id)
		storageInstanceOps, err := removeStorageInstanceOps(st, owner, tag, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, storageInstanceOps...)
	}
	return ops, nil
}

// storageConstraintsDoc contains storage constraints for an entity.
type storageConstraintsDoc struct {
	DocID       string                        `bson:"_id"`
	ModelUUID   string                        `bson:"model-uuid"`
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

func replaceStorageConstraintsOp(key string, cons map[string]StorageConstraints) txn.Op {
	return txn.Op{
		C:      storageConstraintsC,
		Id:     key,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"constraints", cons}}}},
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
		return nil, errors.NotFoundf("storage constraints for %q", key)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage constraints for %q", key)
	}
	return doc.Constraints, nil
}

func storageKind(storageType charm.StorageType) storage.StorageKind {
	kind := storage.StorageKindUnknown
	switch storageType {
	case charm.StorageBlock:
		kind = storage.StorageKindBlock
	case charm.StorageFilesystem:
		kind = storage.StorageKindFilesystem
	}
	return kind
}

func validateStorageConstraints(st *State, allCons map[string]StorageConstraints, charmMeta *charm.Meta) error {
	err := validateStorageConstraintsAgainstCharm(st, allCons, charmMeta)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have constraints specified. Defaults should have
	// been set by this point, if the user didn't specify constraints.
	for name, charmStorage := range charmMeta.Storage {
		if _, ok := allCons[name]; !ok && charmStorage.CountMin > 0 {
			return errors.Errorf("no constraints specified for store %q", name)
		}
	}
	return nil
}

func validateStorageConstraintsAgainstCharm(
	st *State,
	allCons map[string]StorageConstraints,
	charmMeta *charm.Meta,
) error {
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
		if charmStorage.MinimumSize > 0 && cons.Size < charmStorage.MinimumSize {
			return errors.Errorf(
				"charm %q store %q: minimum storage size is %s, %s specified",
				charmMeta.Name, name,
				humanize.Bytes(charmStorage.MinimumSize*humanize.MByte),
				humanize.Bytes(cons.Size*humanize.MByte),
			)
		}
		kind := storageKind(charmStorage.Type)
		if err := validateStoragePool(st, cons.Pool, kind, nil); err != nil {
			return err
		}
	}
	return nil
}

// validateStoragePool validates the storage pool for the model.
// If machineId is non-nil, the storage scope will be validated against
// the machineId; if the storage is not machine-scoped, then the machineId
// will be updated to "".
func validateStoragePool(
	st *State, poolName string, kind storage.StorageKind, machineId *string,
) error {
	if poolName == "" {
		return errors.New("pool name is required")
	}
	providerType, provider, err := poolStorageProvider(st, poolName)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure the storage provider supports the specified kind.
	kindSupported := provider.Supports(kind)
	if !kindSupported && kind == storage.StorageKindFilesystem {
		// Filesystems can be created if either filesystem
		// or block storage are supported.
		if provider.Supports(storage.StorageKindBlock) {
			kindSupported = true
			// The filesystem is to be backed by a volume,
			// so the filesystem must be managed on the
			// machine. Skip the scope-check below by
			// setting the pointer to nil.
			machineId = nil
		}
	}
	if !kindSupported {
		return errors.Errorf("%q provider does not support %q storage", providerType, kind)
	}

	// Check the storage scope.
	if machineId != nil {
		switch provider.Scope() {
		case storage.ScopeMachine:
			if *machineId == "" {
				return errors.Annotate(err, "machine unspecified for machine-scoped storage")
			}
		default:
			// The storage is not machine-scoped, so we clear out
			// the machine ID to inform the caller that the storage
			// scope should be the model.
			*machineId = ""
		}
	}

	return nil
}

func poolStorageProvider(st *State, poolName string) (storage.ProviderType, storage.Provider, error) {
	registry, err := st.storageProviderRegistry()
	if err != nil {
		return "", nil, errors.Annotate(err, "getting storage provider registry")
	}
	poolManager := poolmanager.New(NewStateSettings(st), registry)
	pool, err := poolManager.Get(poolName)
	if errors.IsNotFound(err) {
		// If there's no pool called poolName, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolName)
		provider, err1 := registry.StorageProvider(providerType)
		if err1 != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return "", nil, errors.Trace(err)
		}
		return providerType, provider, nil
	} else if err != nil {
		return "", nil, errors.Trace(err)
	}
	providerType := pool.Provider()
	provider, err := registry.StorageProvider(providerType)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	return providerType, provider, nil
}

// ErrNoDefaultStoragePool is returned when a storage pool is required but none
// is specified nor available as a default.
var ErrNoDefaultStoragePool = fmt.Errorf("no storage pool specifed and no default available")

// addDefaultStorageConstraints fills in default constraint values, replacing any empty/missing values
// in the specified constraints.
func addDefaultStorageConstraints(st *State, allCons map[string]StorageConstraints, charmMeta *charm.Meta) error {
	conf, err := st.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}

	for name, charmStorage := range charmMeta.Storage {
		cons, ok := allCons[name]
		if !ok {
			if charmStorage.Shared {
				// TODO(axw) get the model's default shared storage
				// pool, and create constraints here.
				return errors.Errorf(
					"no constraints specified for shared charm storage %q",
					name,
				)
			}
		}
		cons, err := storageConstraintsWithDefaults(conf, charmStorage, name, cons)
		if err != nil {
			return errors.Trace(err)
		}
		// Replace in case pool or size were updated.
		allCons[name] = cons
	}
	return nil
}

// storageConstraintsWithDefaults returns a constraints
// derived from cons, with any defaults filled in.
func storageConstraintsWithDefaults(
	cfg *config.Config,
	charmStorage charm.Storage,
	name string,
	cons StorageConstraints,
) (StorageConstraints, error) {
	withDefaults := cons

	// If no pool is specified, determine the pool from the env config and other constraints.
	if cons.Pool == "" {
		kind := storageKind(charmStorage.Type)
		poolName, err := defaultStoragePool(cfg, kind, cons)
		if err != nil {
			return withDefaults, errors.Annotatef(err, "finding default pool for %q storage", name)
		}
		withDefaults.Pool = poolName
	}

	// If no size is specified, we default to the min size specified by the
	// charm, or 1GiB.
	if cons.Size == 0 {
		if charmStorage.MinimumSize > 0 {
			withDefaults.Size = charmStorage.MinimumSize
		} else {
			withDefaults.Size = 1024
		}
	}
	if cons.Count == 0 {
		withDefaults.Count = uint64(charmStorage.CountMin)
	}
	return withDefaults, nil
}

// defaultStoragePool returns the default storage pool for the model.
// The default pool is either user specified, or one that is registered by the provider itself.
func defaultStoragePool(cfg *config.Config, kind storage.StorageKind, cons StorageConstraints) (string, error) {
	switch kind {
	case storage.StorageKindBlock:
		loopPool := string(provider.LoopProviderType)

		emptyConstraints := StorageConstraints{}
		if cons == emptyConstraints {
			// No constraints at all: use loop.
			return loopPool, nil
		}
		// Either size or count specified, use env default.
		defaultPool, ok := cfg.StorageDefaultBlockSource()
		if !ok {
			defaultPool = loopPool
		}
		return defaultPool, nil

	case storage.StorageKindFilesystem:
		rootfsPool := string(provider.RootfsProviderType)
		emptyConstraints := StorageConstraints{}
		if cons == emptyConstraints {
			return rootfsPool, nil
		}

		// TODO(axw) add env configuration for default
		// filesystem source, prefer that.
		defaultPool, ok := cfg.StorageDefaultBlockSource()
		if !ok {
			defaultPool = rootfsPool
		}
		return defaultPool, nil
	}
	return "", ErrNoDefaultStoragePool
}

// AddStorageForUnit adds storage instances to given unit as specified.
//
// Missing storage constraints are populated based on model defaults.
// Storage store name is used to retrieve existing storage instances
// for this store. Combination of existing storage instances and
// anticipated additional storage instances is validated against the
// store as specified in the charm.
func (st *State) AddStorageForUnit(
	tag names.UnitTag, name string, cons StorageConstraints,
) error {
	u, err := st.Unit(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := u.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		return st.addStorageForUnitOps(u, name, cons)
	}
	if err := st.run(buildTxn); err != nil {
		return errors.Annotatef(err, "adding storage to unit %s", u)
	}
	return nil
}

// addStorage adds storage instances to given unit as specified.
func (st *State) addStorageForUnitOps(
	u *Unit,
	storageName string,
	cons StorageConstraints,
) ([]txn.Op, error) {
	if u.Life() != Alive {
		return nil, unitNotAliveErr
	}

	// Storage addition is based on the charm metadata, so make sure that
	// the charm URL for the unit or application does not change during
	// the transaction. If the unit does not have a charm URL set yet,
	// then we use the application's charm URL.
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.Name,
		Assert: bson.D{{"charmurl", u.doc.CharmURL}},
	}}
	curl, ok := u.CharmURL()
	if !ok {
		a, err := u.Application()
		if err != nil {
			return nil, errors.Annotatef(err, "getting application for unit %v", u.doc.Name)
		}
		curl = a.doc.CharmURL
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     a.doc.Name,
			Assert: bson.D{{"charmurl", curl}},
		})
	}
	ch, err := st.Charm(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmMeta := ch.Meta()
	charmStorageMeta, ok := charmMeta.Storage[storageName]
	if !ok {
		return nil, errors.NotFoundf("charm storage %q", storageName)
	}

	// Populate missing configuration parameters with default values.
	modelConfig, err := st.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	completeCons, err := storageConstraintsWithDefaults(
		modelConfig,
		charmStorageMeta,
		storageName,
		cons,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This can happen for charm stores that specify instances range from 0,
	// and no count was specified at deploy as storage constraints for this store,
	// and no count was specified to storage add as a contraint either.
	if cons.Count == 0 {
		return nil, errors.NotValidf("adding storage where instance count is 0")
	}

	addUnitStorageOps, err := st.addUnitStorageOps(charmMeta, u, storageName, completeCons, -1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, addUnitStorageOps...)
	return ops, nil
}

// addUnitStorageOps returns transaction ops to create storage for the given
// unit. If countMin is non-negative, the Count field of the constraints will
// be ignored, and as many storage instances as necessary to make up the
// shortfall will be created.
func (st *State) addUnitStorageOps(
	charmMeta *charm.Meta,
	u *Unit,
	storageName string,
	cons StorageConstraints,
	countMin int,
) ([]txn.Op, error) {
	currentCountOp, currentCount, err := st.countEntityStorageInstances(u.Tag(), storageName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := []txn.Op{currentCountOp}
	if countMin >= 0 {
		if currentCount >= countMin {
			return ops, nil
		}
		cons.Count = uint64(countMin - currentCount)
	}

	consTotal := cons
	consTotal.Count += uint64(currentCount)
	if err := validateStorageConstraintsAgainstCharm(st,
		map[string]StorageConstraints{storageName: consTotal},
		charmMeta,
	); err != nil {
		return nil, errors.Trace(err)
	}

	// Create storage db operations
	storageOps, _, err := createStorageOps(
		st,
		u.Tag(),
		charmMeta,
		map[string]StorageConstraints{storageName: cons},
		u,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, txn.Op{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$inc",
			bson.D{{"storageattachmentcount", int(cons.Count)}}}},
	})
	return append(ops, storageOps...), nil
}

func (st *State) countEntityStorageInstances(owner names.Tag, name string) (txn.Op, int, error) {
	refcounts, closer := st.getCollection(refcountsC)
	defer closer()
	key := entityStorageRefcountKey(owner, name)
	return nsRefcounts.CurrentOp(refcounts, key)
}
