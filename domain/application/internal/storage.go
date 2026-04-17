// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
)

// StorageDirective defines a storage directive that already exists for either
// an application or unit.
type StorageDirective struct {
	// CharmMetadataName is the metadata name of the charm the directive exists
	// for.
	CharmMetadataName string

	// Count represents the number of storage instances that should be made for
	// this directive. This value should be the desired count but not the limit.
	// For the maximum supported limit see [StorageDirective.MaxCount].
	Count uint32

	// CharmStorageType represents the storage type of the charm that the
	// directive relates to.
	CharmStorageType domainapplicationcharm.StorageType

	// MaxCount represents the maximum number of storage instances that can be
	// made for this directive. If [domainapplicationcharm.StorageNoMaxCount] is
	// the value, it means that no maximum exists for the storage directive.
	MaxCount int

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive.
	PoolUUID domainstorage.StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

// StorageInfoForAdd represents the arguments required to
// add storage to a unit.
type StorageInfoForAdd struct {
	// CharmStorageDefinitionForValidation holds the storage definition
	// information from the Unit's charm for the purpose of validating against.
	CharmStorageDefinitionForValidation

	// AlreadyAttachedCount is the count of attached Storage Instances this Unit
	// already has for the Charm storage definition.
	AlreadyAttachedCount uint32
}

// StorageInstanceUnitAttachment identifies an attachment of a storage instance
// to a unit.
type StorageInstanceUnitAttachment struct {
	// UnitUUID is the UUID of the unit that is attached to the storage instance.
	UnitUUID coreunit.UUID
	// UUID is the unique identifier of the storage instance attachment.
	UUID domainstorage.StorageAttachmentUUID
}

// StorageInstanceInfoForAttach represents information about a storage instance
// in the model that is suitable for making attachment decisions. This differs
// from [StorageInstanceInfoForUnitAttach] as it does not include unit
// information. Callers are expected to supply unit context separately.
type StorageInstanceInfoForAttach struct {
	// StorageInstanceInfo holds details about the storage instance itself and
	// its backing storage.
	StorageInstanceInfo

	// StorageInstanceAttachments lists existing unit attachments for the
	// storage instance, used to detect conflicts or duplicates.
	StorageInstanceAttachments []StorageInstanceUnitAttachment
}

// StorageInstanceInfoForUnitAttach represents the information required to make
// a decision on if a storage instance can be attached to a unit.
type StorageInstanceInfoForUnitAttach struct {
	// StorageInstanceInfo holds details about the storage instance itself and
	// its backing storage.
	StorageInstanceInfo

	// UnitNamedStorageInfo holds the unit and charm storage definition metadata
	// used to validate the attach operation.
	UnitNamedStorageInfo

	// StorageInstanceAttachments lists existing unit attachments for the
	// Storage Instance, used to detect conflicts or duplicates.
	StorageInstanceAttachments []StorageInstanceUnitAttachment
}

// UnitNamedStorageInfo describes information about a named storage definition
// of a Unit's charm including the unit itself. This information is used to
// validate further storage operations on a Unit.
type UnitNamedStorageInfo struct {
	// CharmStorageDefinitionForValidation holds the charm storage definition
	// used to validate unit storage operations.
	CharmStorageDefinitionForValidation

	// UUID is the unique identifier of the Unit.
	UUID coreunit.UUID

	// AlreadyAttachedCount is the count of storage instances already attached
	// to this unit for the named charm storage definition.
	AlreadyAttachedCount uint32

	// CharmMetadataName is the metadata name of the charm that the unit is
	// running.
	CharmMetadataName string

	// CharmUUID is the unique identifier of the charm that the unit is running.
	CharmUUID corecharm.ID

	// Life is the current lifecycle value of the Unit.
	Life domainlife.Life

	// MachineUUID is the UUID of the machine that the Unit is running on.
	MachineUUID *coremachine.UUID

	// Name is the name of the unit (for example, mysql/0).
	Name coreunit.Name

	// NetNodeUUID is the network node uuid associated with the Unit.
	NetNodeUUID domainnetwork.NetNodeUUID
}

// StorageInstanceFilesystemInfo describes the Filesystem backing details for a
// storage instance.
type StorageInstanceFilesystemInfo struct {
	// UUID is the unique identifier of the backing Filesystem.
	UUID domainstorage.FilesystemUUID

	// ProvisionScope is the provision scope of the backing Filesystem.
	ProvisionScope domainstorage.ProvisionScope

	// SizeMib is the provisioned size of the backing Filesystem in MiB. When
	// the Filesystem has not yet been provisioned this value will be 0.
	SizeMib uint64

	// OwningMachineUUID is the machine that owns this Filesystem, if any.
	// The Filesystem when owned by a machine can only be attached to units on
	// the same machine.
	OwningMachineUUID *coremachine.UUID
}

// StorageInstanceInfo describes a storage instance and its backing storage
// details.
type StorageInstanceInfo struct {
	// UUID is the unique identifier of the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// CharmName represents the metadata name of the charm that the Storage
	// Instance is supposed to be associated with. When not set the charm
	// association has not been established as yet. This would be expected
	// for Storage Instances that have been imported externally into the model.
	CharmName *string

	// Filesystem contains backing Filesystem details when this is Filesystem
	// Storage Instance. It is nil for non-filesystem storage.
	Filesystem *StorageInstanceFilesystemInfo

	// Kind is the storage kind for the Storage Instance.
	Kind domainstorage.StorageKind

	// Life is the current lifecycle value for the Storage Instance.
	Life domainlife.Life

	// RequestedSizeMIB is the requested size of the Storage Instance in MiB.
	RequestedSizeMIB uint64

	// StorageName is the charm storage definition name for this instance.
	// This value will always be set and available.
	StorageName string

	// Volume contains backing volume details when this Storage Instance is
	// either block storage or a Volume backed Filesystem. Nil when the Storage
	// Instance has no volume.
	Volume *StorageInstanceVolumeInfo
}

// StorageInstanceVolumeInfo describes the Volume backing details for a Storage
// Instance.
type StorageInstanceVolumeInfo struct {
	// UUID is the unique identifier of the backing Volume.
	UUID domainstorage.VolumeUUID

	// ProvisionScope is the provision scope of the backing Volume.
	ProvisionScope domainstorage.ProvisionScope

	// SizeMiB is the provisioned size of the backing Volume in MiB. When the
	// volume has not yet been provisioned this value will be 0.
	SizeMiB uint64

	// OwningMachineUUID is the machine that owns this volume, if any. The
	// Volume when owned by a machine can only be attached to units on the
	// same machine.
	OwningMachineUUID *coremachine.UUID
}

// CreateStorageInstanceAttachmentArg describes the arguments required for
// creating a new Storage Instance attachment.
type CreateStorageInstanceAttachmentArg struct {
	// UUID is the unique identifier to associate with the storage attachment.
	UUID domainstorage.StorageAttachmentUUID

	// FilesystemAttachment describes a filesystem to attach for the storage
	// instance attachment.
	FilesystemAttachment *CreateUnitStorageFilesystemAttachmentArg

	// StorageInstanceUUID is the unique identifier of the storage instance
	// to attach to the unit.
	StorageInstanceUUID domainstorage.StorageInstanceUUID

	// VolumeAttachment describes a volume to attach for the storage
	// instance attachment.
	VolumeAttachment *CreateUnitStorageVolumeAttachmentArg
}

// UnitStorageInstanceAttachmentCheckArgs describes the expected state of a
// unit when attaching an existing storage instance.
type UnitStorageInstanceAttachmentCheckArgs struct {
	// CountLessThanEqual is the maximum storage count allowed at the time
	// the add is performed in order for the attach operation to be considered
	// successful.
	CountLessThanEqual uint32

	// CharmUUID is the expected charm UUID that the unit is using. If the
	// unit's charm UUID is different then it indicates the the unit has been
	// upgraded.
	//
	// This would result in the assumptions made about attaching an existing
	// Storage Instance to a unit invalid.
	CharmUUID corecharm.ID

	// MachineUUID is the expected machine UUID for the unit receiving the
	// attachment. If not set it is expected that the unit is not assigned to a
	// Machine.
	MachineUUID *coremachine.UUID
}

// StorageInstanceAttachmentCheckArgs describes the expected storage attachment
// uuids for a Storage Instance. This set of args as a check to assert that
// pre-condition about the state of a Storage Instance remains the same.
type StorageInstanceAttachmentCheckArgs struct {
	// ExpectedAttachments is the set of storage attachment UUIDs expected to
	// exist for the storage instance. These values must be ensured to be unique
	// by the caller.
	ExpectedAttachments []domainstorage.StorageAttachmentUUID

	// UUID is the unique identifier of the Storage Instance to check
	// attachments against.
	UUID domainstorage.StorageInstanceUUID
}

// CreateUnitStorageFilesystemArg describes a set of arguments for a filesystem
// that should be created as part of a unit's storage.
type CreateUnitStorageFilesystemArg struct {
	// UUID describes the unique identifier of the filesystem to
	// create alongside the storage instance.
	UUID domainstorage.FilesystemUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem.
	ProvisionScope domainstorage.ProvisionScope
}

// CreateUnitStorageFilesystemAttachmentArg describes a set of arguments for a
// filesystem attachment that should be created alongside a unit's storage in
// the model.
type CreateUnitStorageFilesystemAttachmentArg struct {
	// FilesystemUUID is the unique identifier of the filesystem to be attached.
	FilesystemUUID domainstorage.FilesystemUUID

	// NetNodeUUID is the net node of the model entity that filesystem will be
	// attached to.
	NetNodeUUID domainnetwork.NetNodeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem attachment.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique identifier to give the filesystem attachment in the
	// model.
	UUID domainstorage.FilesystemAttachmentUUID
}

// CreateUnitStorageInstanceArg describes a set of arguments that create a new
// storage instance on behalf of a unit.
type CreateUnitStorageInstanceArg struct {
	// CharmName is the name of the charm that this storage instance is being
	// provisioned for. This value helps Juju later identify what charm this
	// storage can be re-attached back to.
	CharmName string

	// Filesystem describes the properties of a new filesystem to be created
	// alongside the  storage instance. If this value is not nil a new
	// filesystem will be created with the storage instance.
	Filesystem *CreateUnitStorageFilesystemArg

	// Kind defines the type of storage that is being created.
	Kind domainstorage.StorageKind

	// Name is the name of the storage and must correspond to the storage name
	// defined in the charm the unit is running.
	Name domainstorage.Name

	// RequestSizeMiB defines the requested size of this storage instance in
	// MiB. What ends up being allocated for the storage instance will be at
	// least this value.
	RequestSizeMiB uint64

	// StoragePoolUUID is the pool for which this storage instance is to be
	// provisioned from.
	StoragePoolUUID domainstorage.StoragePoolUUID

	// Volume describes the properties of a new volume to be created alongside
	// the storage instance. If this value is not nil a new volume will be
	// created with the storage instance.
	Volume *CreateUnitStorageVolumeArg

	// UUID is the unique identifier to associate with the storage instance.
	UUID domainstorage.StorageInstanceUUID
}

// CreateUnitStorageVolumeArg describes a set of arguments for a volume
// that should be created as part of a unit's storage.
type CreateUnitStorageVolumeArg struct {
	// UUID describes the unique identifier of the volume to
	// create alongside the storage instance.
	UUID domainstorage.VolumeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created volume.
	ProvisionScope domainstorage.ProvisionScope
}

// CreateUnitStorageVolumeAttachmentArg describes a set of arguments for a
// volume attachment that should be created alongside a unit's storage in
// the model.
type CreateUnitStorageVolumeAttachmentArg struct {
	// NetNodeUUID is the net node of the model entity that volume will be
	// attached to.
	NetNodeUUID domainnetwork.NetNodeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem attachment.
	ProvisionScope domainstorage.ProvisionScope

	// VolumeUUID is the unique identifier of the volume to be attached.
	VolumeUUID domainstorage.VolumeUUID

	// UUID is the unique identifier to give the volume attachment in the
	// model.
	UUID domainstorage.VolumeAttachmentUUID

	// ProviderID if set, forms the pre-determined volume attachment
	// provider id.
	ProviderID *string
}

// AddStorageInstanceArg describes a set of arguments used
// to add a unit storage instance.
type AddStorageInstanceArg struct {
	// Filesystem describes the properties of a new filesystem to be created
	// alongside the  storage instance. If this value is not nil a new
	// filesystem will be created with the storage instance.
	Filesystem *CreateUnitStorageFilesystemArg

	// Volume describes the properties of a new volume to be created alongside
	// the storage instance. If this value is not nil a new volume will be
	// created with the storage instance.
	Volume *CreateUnitStorageVolumeArg

	// UUID is the unique identifier of the storage instance.
	UUID domainstorage.StorageInstanceUUID
}

// AttachStorageInstanceArg describes a set of arguments used
// to attach a unit storage instance.
type AttachStorageInstanceArg AddStorageInstanceArg

// ModelStoragePools provides the default storage pools that have been set
// within the model. If a value is nil then no default exists.
type ModelStoragePools struct {
	// BlockDevicePoolUUID provides the storage pool uuid to use for new block
	// storage.
	BlockDevicePoolUUID *domainstorage.StoragePoolUUID

	// FilesystemPoolUUID provides the storage pool uuid to use for
	// filesystem storage.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID
}

// StorageInstanceComposition describes the composition of a storage instance
// with in the model. This information is required for attaching existing
// storage in the model to a unit. To be able to properly generate attachments
// this information is required.
type StorageInstanceComposition struct {
	// Filesystem when non-nil describes the filesystem information that is part
	// of the storage composition.
	Filesystem *StorageInstanceCompositionFilesystem

	// StorageName is the name of the storage instance and can be considered to
	// be directly related to the charm storage for which it was provisioned.
	StorageName domainstorage.Name

	// UUID is the unique id of the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// Volume when non nil describes the volume information that is part of the
	// storage composition.
	Volume *StorageInstanceCompositionVolume
}

// StorageAttachmentComposition describes the composition of a storage
// attachment with in the model. This information is required for (re-)attaching
// existing storage in the model to a unit. To be able to properly generate
// attachments this information is required.
type StorageAttachmentComposition struct {
	// UUID is the unique id of the storage attachment.
	UUID domainstorage.StorageAttachmentUUID

	// StorageInstanceUUID is the unique id of the storage instance.
	StorageInstanceUUID domainstorage.StorageInstanceUUID

	// FilesystemAttachments is the filesystem attachment information that
	// is part of the storage attachment composition.
	FilesystemAttachment *StorageInstanceCompositionFilesystemAttachment

	// VolumeAttachments is the volume attachment information that is part
	// of the storage attachment composition.
	VolumeAttachment *StorageInstanceCompositionVolumeAttachment
}

// StorageInstanceCompositionFilesystem describes the filesystem information
// that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionFilesystem struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this filesystem.
	ProviderID string

	// ProvisionScope is the provision scope of the filesystem that is
	// attached to this storage instance. This value is only considered valid
	// when [StorageInstanceComposition.FilesystemUUID] is not nil.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the filesystem that is associated with
	// this storage instance. If the value is nil then no filesystem exists.
	UUID domainstorage.FilesystemUUID
}

// StorageInstanceCompositionVolume describes the volume information that is
// part of a [StorageInstanceComposition].
type StorageInstanceCompositionVolume struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this volume.
	ProviderID string

	// ProvisionScope is the provision scope of the volume that is
	// attached to this storage instance. This value is only considered valid
	// when [StorageInstanceComposition.VolumeUUID] is not nil.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the volume that is associated with this
	// storage instance. If the value is nil then no volume exists.
	UUID domainstorage.VolumeUUID
}

// StorageInstanceCompositionFilesystemAttachment describes the filesystem
// attachment information that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionFilesystemAttachment struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this filesystem attachment.
	ProviderID string

	// ProvisionScope is the provision scope of the filesystem attachment that
	// is attached to this storage instance.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the filesystem attachment that is associated
	// with this storage instance.
	UUID domainstorage.FilesystemAttachmentUUID

	// FilesystemUUID is the unique id of the filesystem that is associated
	// with this filesystem attachment.
	FilesystemUUID domainstorage.FilesystemUUID
}

// StorageInstanceCompositionVolumeAttachment describes the volume information
// that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionVolumeAttachment struct {
	// ProviderID is the unique id assigned by the storage pool provider for
	// this volume attachment.
	ProviderID string

	// ProvisionScope is the provision scope of the volume attachment that is
	// attached to this storage instance.
	ProvisionScope domainstorage.ProvisionScope

	// UUID is the unique id of the volume attachment that is associated with
	// this storage instance.
	UUID domainstorage.VolumeAttachmentUUID

	// VolumeUUID is the unique id of the volume that is associated with this
	// volume attachment.
	VolumeUUID domainstorage.VolumeUUID
}

// UnitStorageRefreshArgs describes the required arguments to refresh a unit
// to use a new charm with new storage.
type UnitStorageRefreshArgs struct {
	// NetNodeUUID is the net node of the unit.
	NetNodeUUID domainnetwork.NetNodeUUID

	// MachineUUID is not nil when this unit exists on a machine.
	MachineUUID *coremachine.UUID

	// CurrentCharmUUID is the uuid of the current charm the unit is using.
	CurrentCharmUUID corecharm.ID

	// RefreshCharmUUID is the uuid of the refresh charm the unit will use.
	RefreshCharmUUID corecharm.ID

	// RefreshStorageDirectives is the storage directives when the unit uses the
	// charm specified in [RefreshCharmUUID].
	RefreshStorageDirectives []StorageDirective
}

// StorageInstanceCharmNameSetArg describes the arguments required for
// updating a Storage Instance charm name. This is done when a Storage Instance
// is imported to a model and being attached to a unit to fulfill a Charm's
// storage definition for the first time.
type StorageInstanceCharmNameSetArg struct {
	// CharmMetadataName is the charm name to associate with the storage instance.
	CharmMetadataName string

	// UUID is the unique identifier of the Storage Instance to update.
	UUID domainstorage.StorageInstanceUUID
}
