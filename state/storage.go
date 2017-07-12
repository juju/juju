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
	// instance, and a boolean indicating whether or not there is an owner.
	//
	// When a non-shared storage instance is detached from the unit, the
	// storage instance's owner will be cleared, allowing it to be attached
	// to another unit.
	Owner() (names.Tag, bool)

	// StorageName returns the name of the storage, as defined in the charm
	// storage metadata. This does not uniquely identify storage instances,
	// but identifies the group that the instances belong to.
	StorageName() string

	// Life reports whether the storage instance is Alive, Dying or Dead.
	Life() Life

	// Pool returns the name of the storage pool from which the storage
	// instance has been or will be provisioned.
	Pool() string
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

func (s *storageInstance) Owner() (names.Tag, bool) {
	owner := s.maybeOwner()
	return owner, owner != nil
}

func (s *storageInstance) maybeOwner() names.Tag {
	if s.doc.Owner == "" {
		return nil
	}
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

func (s *storageInstance) Pool() string {
	return s.doc.Constraints.Pool
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

	Id              string                     `bson:"id"`
	Kind            StorageKind                `bson:"storagekind"`
	Life            Life                       `bson:"life"`
	Owner           string                     `bson:"owner,omitempty"`
	StorageName     string                     `bson:"storagename"`
	AttachmentCount int                        `bson:"attachmentcount"`
	Constraints     storageInstanceConstraints `bson:"constraints"`
}

// storageInstanceConstraints contains a subset of StorageConstraints,
// for a single storage instance.
type storageInstanceConstraints struct {
	Pool string `bson:"pool"`
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
func newStorageInstanceId(mb modelBackend, store string) (string, error) {
	seq, err := sequence(mb, "stores")
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
	storageInstances, cleanup := st.db().GetCollection(storageInstancesC)
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
func (st *State) AllStorageInstances() ([]StorageInstance, error) {
	storageInstances, err := st.storageInstances(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]StorageInstance, len(storageInstances))
	for i, s := range storageInstances {
		out[i] = s
	}
	return out, nil
}

func (st *State) storageInstances(query bson.D) (storageInstances []*storageInstance, err error) {
	storageCollection, closer := st.db().GetCollection(storageInstancesC)
	defer closer()

	sdocs := []storageInstanceDoc{}
	err = storageCollection.Find(query).All(&sdocs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get storage instances")
	}
	for _, doc := range sdocs {
		storageInstances = append(storageInstances, &storageInstance{st, doc})
	}
	return storageInstances, nil
}

// DestroyStorageInstance ensures that the storage instance and all its
// attachments will be removed at some point; if the storage instance has
// no attachments, it will be removed immediately.
func (st *State) DestroyStorageInstance(tag names.StorageTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy storage %q", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageInstance(tag)
		if errors.IsNotFound(err) && attempt > 0 {
			// On the first attempt, we expect it to exist.
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
	return st.db().Run(buildTxn)
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
		return removeStorageInstanceOps(s, assert)
	}

	// Check that removing the storage from its owner (if any) is permitted.
	owner := s.maybeOwner()
	var validateRemoveOps []txn.Op
	var ownerAssert bson.DocElem
	if owner != nil {
		var err error
		validateRemoveOps, err = validateRemoveOwnerStorageInstanceOps(s)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ownerAssert = bson.DocElem{"owner", owner.String()}
	} else {
		ownerAssert = bson.DocElem{"owner", bson.D{{"$exists", false}}}
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
	}
	ops = append(ops, validateRemoveOps...)
	ops = append(ops, txn.Op{
		C:      storageInstancesC,
		Id:     s.doc.Id,
		Assert: append(notLastRefs, ownerAssert),
		Update: update,
	})
	return ops, nil
}

// removeStorageInstanceOps removes the storage instance with the given
// tag from state, if the specified assertions hold true.
func removeStorageInstanceOps(
	si *storageInstance,
	assert bson.D,
) ([]txn.Op, error) {

	// Remove the storage instance document, ensuring the owner does not
	// change from what's passed in.
	owner := si.maybeOwner()
	var ownerAssert bson.DocElem
	if owner != nil {
		ownerAssert = bson.DocElem{"owner", owner.String()}
	} else {
		ownerAssert = bson.DocElem{"owner", bson.D{{"$exists", false}}}
	}
	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     si.doc.Id,
		Assert: append(assert, ownerAssert),
		Remove: true,
	}}
	if owner != nil {
		// Ensure that removing the storage will not violate the
		// owner's charm storage requirements.
		validateRemoveOps, err := validateRemoveOwnerStorageInstanceOps(si)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, validateRemoveOps...)

		// Decrement the owner's count for the storage name, freeing
		// up a slot for a new storage instance to be attached.
		decrefOp, err := decrefEntityStorageOp(si.st, owner, si.StorageName())
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, decrefOp)
	}

	machineStorageOp := func(c string, id string) txn.Op {
		return txn.Op{
			C:      c,
			Id:     id,
			Assert: bson.D{{"storageid", si.doc.Id}},
			Update: bson.D{{"$unset", bson.D{{"storageid", nil}}}},
		}
	}

	// Destroy any assigned volume/filesystem, and clear the storage
	// reference to avoid a dangling pointer while the volume/filesystem
	// is being destroyed.
	var haveFilesystem bool
	filesystem, err := si.st.storageInstanceFilesystem(si.StorageTag())
	if err == nil {
		ops = append(ops, machineStorageOp(
			filesystemsC, filesystem.Tag().Id(),
		))
		fsOps, err := destroyFilesystemOps(si.st, filesystem, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, fsOps...)
		haveFilesystem = true
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	volume, err := si.st.storageInstanceVolume(si.StorageTag())
	if err == nil {
		ops = append(ops, machineStorageOp(
			volumesC, volume.Tag().Id(),
		))
		// If the storage instance has a filesystem, it may also
		// have a volume (i.e. for volume-backed filesytems). In
		// this case, we want to destroy only the filesystem; when
		// the filesystem is removed, the volume will be destroyed.
		if !haveFilesystem {
			volOps, err := destroyVolumeOps(si.st, volume, nil)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, volOps...)
		}
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return ops, nil
}

// validateRemoveOwnerStorageInstanceOps checks that the given storage
// instance can be removed from its current owner, returning txn.Ops to
// ensure the same in a transaction. If the owner is not alive, then charm
// storage requirements are ignored.
func validateRemoveOwnerStorageInstanceOps(si *storageInstance) ([]txn.Op, error) {
	var ops []txn.Op
	var charmMeta *charm.Meta
	owner := si.maybeOwner()
	switch owner.Kind() {
	case names.ApplicationTagKind:
		app, err := si.st.Application(owner.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if app.Life() != Alive {
			return nil, nil
		}
		ch, _, err := app.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		charmMeta = ch.Meta()
		ops = append(ops, txn.Op{
			C:  applicationsC,
			Id: app.Name(),
			Assert: bson.D{
				{"life", Alive},
				{"charmurl", ch.URL},
			},
		})
	case names.UnitTagKind:
		u, err := si.st.Unit(owner.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if u.Life() != Alive {
			return nil, nil
		}
		ch, err := u.charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		charmMeta = ch.Meta()
		ops = append(ops, txn.Op{
			C:      unitsC,
			Id:     u.doc.Name,
			Assert: bson.D{{"life", Alive}},
		})
		ops = append(ops, u.assertCharmOps(ch)...)
	default:
		return nil, errors.Errorf(
			"invalid storage owner %s",
			names.ReadableString(owner),
		)
	}
	_, currentCountOp, err := validateStorageCountChange(
		si.st, owner, si.StorageName(), -1, charmMeta,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, currentCountOp)
	return ops, nil
}

// validateStorageCountChange validates the desired storage count change,
// and returns the current storage count, and a txn.Op that ensures the
// current storage count does not change before the transaction is executed.
func validateStorageCountChange(
	st *State, owner names.Tag,
	storageName string, n int,
	charmMeta *charm.Meta,
) (current int, _ txn.Op, _ error) {
	currentCountOp, currentCount, err := st.countEntityStorageInstances(owner, storageName)
	if err != nil {
		return -1, txn.Op{}, errors.Trace(err)
	}
	charmStorage := charmMeta.Storage[storageName]
	if err := validateCharmStorageCountChange(charmStorage, currentCount, n); err != nil {
		return -1, txn.Op{}, errors.Trace(err)
	}
	return currentCount, currentCountOp, nil
}

// increfEntityStorageOp returns a txn.Op that increments the reference
// count for a storage instance for a given application or unit. This
// should be called when creating a shared storage instance, or when
// attaching a non-shared storage instance to a unit.
func increfEntityStorageOp(mb modelBackend, owner names.Tag, storageName string, n int) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()
	storageRefcountKey := entityStorageRefcountKey(owner, storageName)
	incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, storageRefcountKey, n)
	return incRefOp, errors.Trace(err)
}

// decrefEntityStorageOp returns a txn.Op that decrements the reference
// count for a storage instance from a given application or unit. This
// should be called when removing a shared storage instance, or when
// detaching a non-shared storage instance from a unit.
func decrefEntityStorageOp(mb modelBackend, owner names.Tag, storageName string) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()
	storageRefcountKey := entityStorageRefcountKey(owner, storageName)
	decRefOp, _, err := nsRefcounts.DyingDecRefOp(refcounts, storageRefcountKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	return decRefOp, nil
}

// machineAssignable is used by createStorageOps to determine what machine
// storage needs to be created. This is implemented by Unit.
type machineAssignable interface {
	machine() (*Machine, error)
	noAssignedMachineOp() txn.Op
}

// createStorageOps returns txn.Ops for creating storage instances
// and attachments for the newly created unit or service. A map
// of storage names to number of storage instances created will
// be returned, along with the total number of storage attachments
// made. These should be used to initialise or update refcounts.
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
	series string,
	maybeMachineAssignable machineAssignable,
) (ops []txn.Op, instanceCounts map[string]int, numStorageAttachments int, err error) {

	fail := func(err error) ([]txn.Op, map[string]int, int, error) {
		return nil, nil, -1, err
	}

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
		return fail(errors.Errorf("expected application or unit tag, got %T", entityTag))
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
			return fail(errors.NotFoundf("charm storage %q", store))
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

	instanceCounts = make(map[string]int)
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
			return fail(errors.Errorf("unknown storage type %q", t.meta.Type))
		}

		instanceCounts[t.storageName] += int(t.cons.Count)
		for i := uint64(0); i < t.cons.Count; i++ {
			cons := cons[t.storageName]
			id, err := newStorageInstanceId(st, t.storageName)
			if err != nil {
				return fail(errors.Annotate(err, "cannot generate storage instance name"))
			}
			doc := &storageInstanceDoc{
				Id:          id,
				Kind:        kind,
				Owner:       owner,
				StorageName: t.storageName,
				Constraints: storageInstanceConstraints{
					Pool: cons.Pool,
					Size: cons.Size,
				},
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
						st, unitTag, charmMeta, series,
						&storageInstance{st, *doc},
						maybeMachineAssignable,
					)
					if err != nil {
						return fail(errors.Annotatef(
							err, "creating machine storage for storage %s", id,
						))
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

	return ops, instanceCounts, numStorageAttachments, nil
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
	series string,
	storage *storageInstance,
	machineAssignable machineAssignable,
) (ops []txn.Op, err error) {
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

	storageParams, err := machineStorageParamsForStorageInstance(
		st, charmMeta, unitTag, series, storage,
	)
	if err != nil {
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
	coll, closer := st.db().GetCollection(storageAttachmentsC)
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
	coll, closer := st.db().GetCollection(storageAttachmentsC)
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

// AttachStorage attaches storage to a unit, creating and attaching machine
// storage as necessary.
func (st *State) AttachStorage(storage names.StorageTag, unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err,
		"cannot attach %s to %s",
		names.ReadableString(storage),
		names.ReadableString(unit),
	)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		si, err := st.storageInstance(storage)
		if err != nil {
			return nil, errors.Trace(err)
		}
		u, err := st.Unit(unit.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if u.Life() != Alive {
			return nil, errors.New("unit not alive")
		}
		ch, err := u.charm()
		if err != nil {
			return nil, errors.Annotate(err, "getting charm")
		}
		ops, err := st.attachStorageOps(si, u.UnitTag(), u.Series(), ch, u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if si.doc.Owner == "" {
			// The storage instance will be owned by the unit, so we
			// must increment the unit's refcount for the storage name.
			//
			// Make sure that we *can* assign another storage instance
			// to the unit.
			_, currentCountOp, err := validateStorageCountChange(
				st, u.UnitTag(), si.StorageName(), 1, ch.Meta(),
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			incRefOp, err := increfEntityStorageOp(st, u.UnitTag(), si.StorageName(), 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, currentCountOp, incRefOp)
		}
		ops = append(ops, txn.Op{
			C:      unitsC,
			Id:     u.doc.Name,
			Assert: isAliveDoc,
			Update: bson.D{{"$inc", bson.D{{"storageattachmentcount", 1}}}},
		})
		ops = append(ops, u.assertCharmOps(ch)...)
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// attachStorageOps returns txn.Ops to attach a storage instance to the
// specified unit. The caller must ensure that the unit is in a state
// to attach the storage (i.e. it is Alive, or is being created).
//
// The caller is responsible for incrementing the storage refcount for
// the unit/storage name.
func (st *State) attachStorageOps(
	si *storageInstance,
	unitTag names.UnitTag,
	unitSeries string,
	ch *Charm,
	maybeMachineAssignable machineAssignable,
) ([]txn.Op, error) {
	if si.Life() != Alive {
		return nil, errors.New("storage not alive")
	}
	unitApplicationName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if owner, ok := si.Owner(); ok {
		if owner == unitTag {
			return nil, jujutxn.ErrNoOperations
		} else {
			if owner.Id() != unitApplicationName {
				return nil, errors.Errorf(
					"cannot attach storage owned by %s to %s",
					names.ReadableString(owner),
					names.ReadableString(unitTag),
				)
			}
			if _, err := st.storageAttachment(
				si.StorageTag(),
				unitTag,
			); err == nil {
				return nil, jujutxn.ErrNoOperations
			} else if !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		}
	} else {
		// TODO(axw) should we store the application name on the
		// storage, and restrict attaching to only units of that
		// application?
	}

	// Check that the unit's charm declares storage with the storage
	// instance's storage name.
	charmMeta := ch.Meta()
	if _, ok := charmMeta.Storage[si.StorageName()]; !ok {
		return nil, errors.Errorf(
			"charm %s has no storage called %s",
			charmMeta.Name, si.StorageName(),
		)
	}

	// Create a storage attachment doc, ensuring that the storage instance
	// owner does not change, and that both the storage instance and unit
	// are alive. Increment the attachment count on both storage instance
	// and unit, and update the owner of the storage instance if necessary.
	siUpdate := bson.D{{"$inc", bson.D{{"attachmentcount", 1}}}}
	siAssert := isAliveDoc
	if si.doc.Owner != "" {
		siAssert = append(siAssert, bson.DocElem{"owner", si.doc.Owner})
	} else {
		siAssert = append(siAssert, bson.DocElem{"owner", bson.D{{"$exists", false}}})
		siUpdate = append(siUpdate, bson.DocElem{
			"$set", bson.D{{"owner", unitTag.String()}},
		})
	}
	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     si.doc.Id,
		Assert: siAssert,
		Update: siUpdate,
	},
		createStorageAttachmentOp(si.StorageTag(), unitTag),
	}

	if maybeMachineAssignable != nil {
		machineStorageOps, err := unitAssignedMachineStorageOps(
			st, unitTag, charmMeta, unitSeries, si,
			maybeMachineAssignable,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, machineStorageOps...)
	}
	return ops, nil
}

// DetachStorage ensures that the existing storage attachments of
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
			ops = append(ops, detachStorageOps(
				attachment.StorageInstance(), unit,
			)...)
		}
		if len(ops) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// DetachStorage ensures that the storage attachment will be
// removed at some point.
func (st *State) DetachStorage(storage names.StorageTag, unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy storage attachment %s:%s", storage.Id(), unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageAttachment(storage, unit)
		if errors.IsNotFound(err) && attempt > 0 {
			// On the first attempt, we expect it to exist.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if s.doc.Life == Dying {
			return nil, jujutxn.ErrNoOperations
		}
		si, err := st.storageInstance(storage)
		if err != nil {
			return nil, jujutxn.ErrNoOperations
		}
		var ops []txn.Op
		var ownerAssert bson.DocElem
		switch owner := si.maybeOwner(); owner {
		case nil:
			ownerAssert = bson.DocElem{"owner", bson.D{{"$exists", false}}}
		case unit:
			validateRemoveOps, err := validateRemoveOwnerStorageInstanceOps(si)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, validateRemoveOps...)
			fallthrough
		default:
			ownerAssert = bson.DocElem{"owner", si.doc.Owner}
		}
		ops = append(ops, txn.Op{
			C:      storageInstancesC,
			Id:     si.doc.Id,
			Assert: bson.D{ownerAssert},
		})

		// Check if the unit is assigned to a machine, and if the
		// associated machine storage has been attached yet. If not,
		// we can short-circuit the removal of the storage attachment.
		var assert interface{}
		removeStorageAttachment := true
		u, err := st.Unit(unit.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		machineId, err := u.AssignedMachineId()
		if errors.IsNotAssigned(err) {
			// The unit is not assigned to a machine, therefore
			// there can be no associated machine storage. It
			// is safe to remove.
			ops = append(ops, u.noAssignedMachineOp())
		} else if err != nil {
			return nil, errors.Trace(err)
		} else {
			machineTag := names.NewMachineTag(machineId)
			volumeAttachment, filesystemAttachment, err := st.storageMachineAttachment(
				si, unit, machineTag,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if volumeAttachment != nil {
				var assert interface{}
				if _, err := volumeAttachment.Info(); err == nil {
					// The volume attachment has been provisioned,
					// so we cannot short-circuit the removal of
					// the storage attachment.
					removeStorageAttachment = false
					assert = txn.DocExists
				} else {
					assert = bson.D{{"info", bson.D{{"$exists", false}}}}
				}
				ops = append(ops, txn.Op{
					C: volumeAttachmentsC,
					Id: volumeAttachmentId(
						volumeAttachment.Machine().Id(),
						volumeAttachment.Volume().Id(),
					),
					Assert: assert,
				})
			}
			if filesystemAttachment != nil {
				var assert interface{}
				if _, err := filesystemAttachment.Info(); err == nil {
					// The filesystem attachment has been provisioned,
					// so we cannot short-circuit the removal of
					// the storage attachment.
					removeStorageAttachment = false
					assert = txn.DocExists
				} else {
					assert = bson.D{{"info", bson.D{{"$exists", false}}}}
				}
				ops = append(ops, txn.Op{
					C: filesystemAttachmentsC,
					Id: filesystemAttachmentId(
						filesystemAttachment.Machine().Id(),
						filesystemAttachment.Filesystem().Id(),
					),
					Assert: assert,
				})
			}
		}
		if removeStorageAttachment {
			// Short-circuit the removal of the storage attachment.
			return removeStorageAttachmentOps(st, s, si, assert, ops...)
		}
		return append(ops, detachStorageOps(storage, unit)...), nil
	}
	return st.db().Run(buildTxn)
}

func detachStorageOps(storage names.StorageTag, unit names.UnitTag) []txn.Op {
	ops := []txn.Op{{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(unit.Id(), storage.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
	return ops
}

func (st *State) storageMachineAttachment(
	si *storageInstance,
	unitTag names.UnitTag,
	machineTag names.MachineTag,
) (VolumeAttachment, FilesystemAttachment, error) {
	switch si.Kind() {
	case StorageKindBlock:
		volume, err := st.storageInstanceVolume(si.StorageTag())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		att, err := st.VolumeAttachment(machineTag, volume.VolumeTag())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		return att, nil, nil

	case StorageKindFilesystem:
		filesystem, err := st.storageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		att, err := st.FilesystemAttachment(machineTag, filesystem.FilesystemTag())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		return nil, att, nil

	default:
		return nil, nil, errors.Errorf("unknown storage type %q", si.Kind())
	}
}

// Remove removes the storage attachment from state, and may remove its storage
// instance as well, if the storage instance is Dying and no other references to
// it exist. It will fail if the storage attachment is not Dying.
func (st *State) RemoveStorageAttachment(storage names.StorageTag, unit names.UnitTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove storage attachment %s:%s", storage.Id(), unit.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		s, err := st.storageAttachment(storage, unit)
		if errors.IsNotFound(err) && attempt > 0 {
			// On the first attempt, we expect it to exist.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if s.doc.Life != Dying {
			return nil, errors.New("storage attachment is not dying")
		}
		inst, err := st.storageInstance(storage)
		if errors.IsNotFound(err) {
			// This implies that the attachment was removed
			// after the call to st.storageAttachment.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := removeStorageAttachmentOps(st, s, inst, bson.D{{"life", Dying}})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

func removeStorageAttachmentOps(
	st *State,
	s *storageAttachment,
	si *storageInstance,
	assert interface{},
	baseOps ...txn.Op,
) ([]txn.Op, error) {
	ops := append(baseOps, txn.Op{
		C:      storageAttachmentsC,
		Id:     storageAttachmentId(s.doc.Unit, s.doc.StorageInstance),
		Assert: assert,
		Remove: true,
	}, txn.Op{
		C:      unitsC,
		Id:     s.doc.Unit,
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"storageattachmentcount", -1}}}},
	})
	var siAssert interface{}
	siUpdate := bson.D{{"$inc", bson.D{{"attachmentcount", -1}}}}
	if si.doc.AttachmentCount == 1 {
		if si.doc.Life == Dying {
			// The storage instance is dying: no more attachments
			// can be added to the instance, so it can be removed.
			hasLastRef := bson.D{{"life", Dying}, {"attachmentcount", 1}}
			siOps, err := removeStorageInstanceOps(si, hasLastRef)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return append(ops, siOps...), nil
		} else if si.doc.Owner == names.NewUnitTag(s.doc.Unit).String() {
			// Ensure that removing the storage will not violate the
			// unit's charm storage requirements.
			siAssert = bson.D{{"owner", si.doc.Owner}}
			validateRemoveOps, err := validateRemoveOwnerStorageInstanceOps(si)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, validateRemoveOps...)

			// Disown the storage instance, so it can be attached
			// to another unit/application.
			siUpdate = append(siUpdate, bson.DocElem{
				"$unset", bson.D{{"owner", nil}},
			})
			decrefOp, err := decrefEntityStorageOp(st, s.Unit(), si.StorageName())
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, decrefOp)
		}
	}
	decrefOp := txn.Op{
		C:      storageInstancesC,
		Id:     si.doc.Id,
		Assert: siAssert,
		Update: siUpdate,
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

	// If the storage instance has an associated volume or
	// filesystem, and the unit is assigned to a machine,
	// detach the volume/filesystem too.
	machineOps, err := st.detachStorageMachineAttachmentOps(si, s.Unit())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, machineOps...)

	return ops, nil
}

func (st *State) detachStorageMachineAttachmentOps(si *storageInstance, unitTag names.UnitTag) ([]txn.Op, error) {
	unit, err := st.Unit(unitTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineId, err := unit.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		return []txn.Op{unit.noAssignedMachineOp()}, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	machineTag := names.NewMachineTag(machineId)

	switch si.Kind() {
	case StorageKindBlock:
		volume, err := st.storageInstanceVolume(si.StorageTag())
		if errors.IsNotFound(err) {
			// The volume has already been removed, so must have
			// already been detached.
			logger.Debugf("%s", err)
			return nil, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if !volume.Detachable() {
			// Non-detachable volumes are left attached to the
			// machine, since the only other option is to destroy
			// them. The user can remove them explicitly, or else
			// leave them to be removed along with the machine.
			logger.Debugf(
				"%s for %s is non-detachable",
				names.ReadableString(volume.Tag()),
				names.ReadableString(si.StorageTag()),
			)
			return nil, nil
		} else if volume.Life() != Alive {
			// The volume is not alive, so either is already
			// or will soon be detached.
			logger.Debugf(
				"%s is %s",
				names.ReadableString(volume.Tag()),
				volume.Life(),
			)
			return nil, nil
		}
		att, err := st.VolumeAttachment(machineTag, volume.VolumeTag())
		if errors.IsNotFound(err) {
			// Since the storage attachment is Dying, it is not
			// possible to create a volume attachment for the
			// machine, associated with the same storage.
			logger.Debugf("%s", err)
			return nil, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if att.Life() != Alive {
			logger.Debugf(
				"%s is detaching from %s",
				names.ReadableString(volume.Tag()),
				names.ReadableString(machineTag),
			)
			return nil, nil
		}
		return detachVolumeOps(machineTag, volume.VolumeTag()), nil

	case StorageKindFilesystem:
		filesystem, err := st.storageInstanceFilesystem(si.StorageTag())
		if errors.IsNotFound(err) {
			// The filesystem has already been removed, so must
			// have already been detached.
			logger.Debugf("%s", err)
			return nil, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if !filesystem.Detachable() {
			// Non-detachable filesystems are left attached to the
			// machine, since the only other option is to destroy
			// them. The user can remove them explicitly, or else
			// leave them to be removed along with the machine.
			logger.Debugf(
				"%s for %s is non-detachable",
				names.ReadableString(filesystem.Tag()),
				names.ReadableString(si.StorageTag()),
			)
			return nil, nil
		} else if filesystem.Life() != Alive {
			logger.Debugf(
				"%s is %s",
				names.ReadableString(filesystem.Tag()),
				filesystem.Life(),
			)
			return nil, nil
		}
		att, err := st.FilesystemAttachment(machineTag, filesystem.FilesystemTag())
		if errors.IsNotFound(err) {
			// Since the storage attachment is Dying, it is not
			// possible to create a volume attachment for the
			// machine, associated with the same storage.
			logger.Debugf("%s", err)
			return nil, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if att.Life() != Alive {
			logger.Debugf(
				"%s is detaching from %s",
				names.ReadableString(filesystem.Tag()),
				names.ReadableString(machineTag),
			)
			return nil, nil
		}
		return detachFilesystemOps(machineTag, filesystem.FilesystemTag()), nil

	default:
		return nil, errors.Errorf("unknown storage type %q", si.Kind())
	}
}

// removeStorageInstancesOps returns the transaction operations to remove all
// storage instances owned by the specified entity.
func removeStorageInstancesOps(st *State, owner names.Tag) ([]txn.Op, error) {
	coll, closer := st.db().GetCollection(storageInstancesC)
	defer closer()

	var docs []storageInstanceDoc
	err := coll.Find(bson.D{{"owner", owner.String()}}).Select(bson.D{{"id", true}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get storage instances for %s", owner)
	}
	ops := make([]txn.Op, 0, len(docs))
	for _, doc := range docs {
		si := &storageInstance{st, doc}
		storageInstanceOps, err := removeStorageInstanceOps(si, nil)
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
	coll, closer := st.db().GetCollection(storageConstraintsC)
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
		if err := validateCharmStorageCount(charmStorage, cons.Count); err != nil {
			return errors.Annotatef(err, "charm %q store %q", charmMeta.Name, name)
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

func validateCharmStorageCountChange(charmStorage charm.Storage, current, n int) error {
	action := "attach"
	absn := n
	if n < 0 {
		action = "detach"
		absn = -absn
	}
	gerund := action + "ing"
	pluralise := ""
	if absn != 1 {
		pluralise = "s"
	}

	count := uint64(current + n)
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("cannot %s, storage is singular", action)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"which is less than the minimum of %d",
			gerund, absn, pluralise, count,
			charmStorage.CountMin,
		)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"exceeding the maximum of %d",
			gerund, absn, pluralise, count,
			charmStorage.CountMax,
		)
	}
	return nil
}

func validateCharmStorageCount(charmStorage charm.Storage, count uint64) error {
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("storage is singular, %d specified", count)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%d instances required, %d specified",
			charmStorage.CountMin, count,
		)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"at most %d instances supported, %d specified",
			charmStorage.CountMax, count,
		)
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
		// or block storage are supported. The scope of the
		// filesystem is the same as the backing volume.
		kindSupported = provider.Supports(storage.StorageKindBlock)
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
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Annotatef(err, "adding %q storage to %s", name, u)
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

	// Storage addition is based on the charm metadata; u.charm()
	// returns txn.Ops that ensure the charm URL does not change
	// during the transaction.
	ch, err := u.charm()
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmMeta := ch.Meta()
	charmStorageMeta, ok := charmMeta.Storage[storageName]
	if !ok {
		return nil, errors.NotFoundf("charm storage %q", storageName)
	}
	ops := u.assertCharmOps(ch)

	if cons.Pool == "" || cons.Size == 0 {
		// Either pool or size, or both, were not specified. Take the
		// values from the unit's recorded storage constraints.
		allCons, err := u.StorageConstraints()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if uCons, ok := allCons[storageName]; ok {
			if cons.Pool == "" {
				cons.Pool = uCons.Pool
			}
			if cons.Size == 0 {
				cons.Size = uCons.Size
			}
		}

		// Populate missing configuration parameters with defaults.
		if cons.Pool == "" || cons.Size == 0 {
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
			cons = completeCons
		}
	}

	// This can happen for charm stores that specify instances range from 0,
	// and no count was specified at deploy as storage constraints for this store,
	// and no count was specified to storage add as a contraint either.
	if cons.Count == 0 {
		return nil, errors.NotValidf("adding storage where instance count is 0")
	}

	addUnitStorageOps, err := st.addUnitStorageOps(charmMeta, u, storageName, cons, -1)
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
	var ops []txn.Op

	consTotal := cons
	if countMin < 0 {
		// Validate that the requested number of storage
		// instances can be added to the unit.
		currentCount, currentCountOp, err := validateStorageCountChange(
			st, u.Tag(), storageName, int(cons.Count), charmMeta,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, currentCountOp)
		consTotal.Count += uint64(currentCount)
	} else {
		currentCountOp, currentCount, err := st.countEntityStorageInstances(u.Tag(), storageName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, currentCountOp)
		if currentCount >= countMin {
			return ops, nil
		}
		cons.Count = uint64(countMin)
	}

	if err := validateStorageConstraintsAgainstCharm(st,
		map[string]StorageConstraints{storageName: consTotal},
		charmMeta,
	); err != nil {
		return nil, errors.Trace(err)
	}

	// Create storage db operations
	storageOps, storageCounts, _, err := createStorageOps(
		st,
		u.Tag(),
		charmMeta,
		map[string]StorageConstraints{storageName: cons},
		u.Series(),
		u,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Increment reference counts for the named storage for each
	// instance we create. We'll use the reference counts to ensure
	// we don't exceed limits when adding storage, and for
	// maintaining model integrity during charm upgrades.
	for name, count := range storageCounts {
		incRefOp, err := increfEntityStorageOp(st, u.Tag(), name, count)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageOps = append(storageOps, incRefOp)
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
	refcounts, closer := st.db().GetCollection(refcountsC)
	defer closer()
	key := entityStorageRefcountKey(owner, name)
	return nsRefcounts.CurrentOp(refcounts, key)
}
