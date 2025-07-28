// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/names/v6"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
)

// StorageInstance represents the state of a unit or application-wide storage
// instance in the model.
type StorageInstance interface {
	Tag() names.Tag

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
// have an attachment for each unit of the application owning the storage instance.
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

// NewStorageBackend creates a backend for managing storage.
func NewStorageBackend(st *State) (*storageBackend, error) {
	sb := &storageBackend{}
	return sb, nil
}

// NewStorageConfigBackend creates a backend for managing storage with a model
// config service.
func NewStorageConfigBackend(
	st *State,
) (*storageConfigBackend, error) {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	return &storageConfigBackend{
		storageBackend: sb,
	}, nil
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error)
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// storageBackend exposes storage-specific state utilities.
type storageBackend struct {
}

// storageConfigBackend augments storageBackend with methods that require model
// config access.
type storageConfigBackend struct {
	*storageBackend
}

type storageInstance struct {
	doc storageInstanceDoc
}

// String returns a human-readable string representing the type.
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
	return nil, false
}

func (s *storageInstance) StorageName() string {
	return s.doc.StorageName
}

func (s *storageInstance) Life() Life {
	return Dead
}

func (s *storageInstance) Pool() string {
	return s.doc.Constraints.Pool
}

// storageInstanceDoc describes a charm storage instance.
type storageInstanceDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	Id              string                     `bson:"id"`
	Kind            StorageKind                `bson:"storagekind"`
	Life            Life                       `bson:"life"`
	Releasing       bool                       `bson:"releasing,omitempty"`
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
	return Dead
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

// StorageInstance returns the StorageInstance with the specified tag.
func (sb *storageBackend) StorageInstance(tag names.StorageTag) (StorageInstance, error) {
	return &storageInstance{}, nil
}

// AllStorageInstances lists all storage instances currently in state
// for this Juju model.
func (sb *storageBackend) AllStorageInstances() ([]StorageInstance, error) {
	out := make([]StorageInstance, 0)
	return out, nil
}

// DestroyStorageInstance ensures that the storage instance will be removed at
// some point, after the cloud storage resources have been destroyed.
//
// If "destroyAttachments" is true, then DestroyStorageInstance will destroy
// any attachments first; if there are no attachments, then the storage instance
// is removed immediately. If "destroyAttached" is instead false and there are
// existing storage attachments, then DestroyStorageInstance will return an error
// satisfying StorageAttachedError.
func (sb *storageBackend) DestroyStorageInstance(tag names.StorageTag, destroyAttachments bool, force bool, maxWait time.Duration) (err error) {
	return
}

// ReleaseStorageInstance ensures that the storage instance will be removed at
// some point, without destroying the cloud storage resources.
//
// If "destroyAttachments" is true, then DestroyStorageInstance will destroy
// any attachments first; if there are no attachments, then the storage instance
// is removed immediately. If "destroyAttached" is instead false and there are
// existing storage attachments, then ReleaseStorageInstance will return an error
// satisfying StorageAttachedError.
func (sb *storageBackend) ReleaseStorageInstance(tag names.StorageTag, destroyAttachments bool, force bool, maxWait time.Duration) (err error) {
	return
}

// StorageAttachments returns the StorageAttachments for the specified storage
// instance.
func (sb *storageBackend) StorageAttachments(storage names.StorageTag) ([]StorageAttachment, error) {
	return nil, nil
}

// UnitStorageAttachments returns the StorageAttachments for the specified unit.
func (sb *storageBackend) UnitStorageAttachments(unit names.UnitTag) ([]StorageAttachment, error) {
	return nil, nil
}

// StorageAttachment returns the StorageAttachment with the specified tags.
func (sb *storageBackend) StorageAttachment(storage names.StorageTag, unit names.UnitTag) (StorageAttachment, error) {
	return &storageAttachment{}, nil
}

// AttachStorage attaches storage to a unit, creating and attaching machine
// storage as necessary.
func (sb *storageConfigBackend) AttachStorage(storage names.StorageTag, unit names.UnitTag) (err error) {
	return nil
}

// DestroyUnitStorageAttachments ensures that the existing storage
// attachments of the specified unit are removed at some point.
func (sb *storageBackend) DestroyUnitStorageAttachments(unit names.UnitTag) (err error) {
	return nil
}

// DetachStorage ensures that the storage attachment will be
// removed at some point.
func (sb *storageBackend) DetachStorage(storage names.StorageTag, unit names.UnitTag, force bool, maxWait time.Duration) (err error) {
	return
}

// RemoveStorageAttachment removes the storage attachment from state, and may
// remove its storage instance as well, if the storage instance is Dying and
// no other references to it exist.
// It will fail if the storage attachment is not Dying.
func (sb *storageBackend) RemoveStorageAttachment(storage names.StorageTag, unit names.UnitTag, force bool) (err error) {
	return
}

// StorageConstraints contains the user-specified constraints for provisioning
// storage instances for an application unit.
type StorageConstraints struct {
	// Pool is the name of the storage pool from which to provision the
	// storage instances.
	Pool string `bson:"pool"`

	// Size is the required size of the storage instances, in MiB.
	Size uint64 `bson:"size"`

	// Count is the required number of storage instances.
	Count uint64 `bson:"count"`
}

// ErrNoDefaultStoragePool is returned when a storage pool is required but none
// is specified nor available as a default.
var ErrNoDefaultStoragePool = fmt.Errorf("no storage pool specified and no default available")

// AddStorageForUnit adds storage instances to given unit as specified.
//
// Missing storage constraints are populated based on model defaults.
// Storage store name is used to retrieve existing storage instances
// for this store. Combination of existing storage instances and
// anticipated additional storage instances is validated against the
// store as specified in the charm.
func (sb *storageConfigBackend) AddStorageForUnit(
	tag names.UnitTag, storageName string, cons StorageConstraints,
) ([]names.StorageTag, error) {
	return nil, nil
}

// AddStorageForUnitOperation returns a ModelOperation for adding storage
// instances to the given unit as specified.
//
// Missing storage constraints are populated based on model defaults.
// Storage store name is used to retrieve existing storage instances
// for this store. Combination of existing storage instances and
// anticipated additional storage instances is validated against the
// store as specified in the charm.
func (sb *storageConfigBackend) AddStorageForUnitOperation(tag names.UnitTag, storageName string, cons StorageConstraints) (ModelOperation, error) {
	return nil, nil
}

// HostVolumeParams holds the parameters for creating a volume and
// attaching it to a new host.
type HostVolumeParams struct {
	Volume     VolumeParams
	Attachment VolumeAttachmentParams
}

// HostFilesystemParams holds the parameters for creating a filesystem
// and attaching it to a new host.
type HostFilesystemParams struct {
	Filesystem FilesystemParams
	Attachment FilesystemAttachmentParams
}
