// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
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
