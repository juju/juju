// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// CreateApplicationStorageDirectiveArg defines an individual storage directive to be
// associated with an application.
type CreateApplicationStorageDirectiveArg = CreateStorageDirectiveArg

// CreateStorageDirectiveArg defines the arguments required to add a storage
// directive to the model.
type CreateStorageDirectiveArg struct {
	// Count represents the number of storage instances that should be made for
	// this directive.
	Count uint32

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive. This is
	// an optional value and if not set it is expected that
	// [ApplicationStorageDirectiveArg.ProviderType] is set.
	PoolUUID domainstorage.StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

// CreateUnitStorageArg represents the arguments required for making storage
// for a unit. This will create and set the unit's storage directives and then
// instantiate the instances and attachments for the units.
type CreateUnitStorageArg struct {
	// StorageDirectives defines the storage directives that should be created
	// for the unit.
	StorageDirectives []CreateUnitStorageDirectiveArg

	// StorageInstances defines the new storage instances that must be created
	// for the unit.
	StorageInstances []CreateUnitStorageInstanceArg

	// StorageToAttach defines the storage instances that should be attached to
	// the unit. New storage instances defined in
	// [CreateUnitStorageArg.StorageInstances] are not automatically attached to
	// the unit and should be included in this list.
	StorageToAttach []CreateUnitStorageAttachmentArg

	// StorageToOwn defines the storage instances that should be owned by the
	// unit.
	StorageToOwn []domainstorage.StorageInstanceUUID
}

// CreateUnitStorageAttachmentArg describes the arguments required for creating a
// storage attachment.
type CreateUnitStorageAttachmentArg struct {
	// UUID is the unique identifier to associate with the storage attachment.
	UUID domainstorageprov.StorageAttachmentUUID

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

// CreateUnitStorageDirectiveArg describes the arguments required for making storage
// directives on a unit.
type CreateUnitStorageDirectiveArg = CreateStorageDirectiveArg

// CreateUnitStorageFilesystemArg describes a set of arguments for a filesystem
// that should be created as part of a unit's storage.
type CreateUnitStorageFilesystemArg struct {
	// UUID describes the unique identifier of the filesystem to
	// create alongside the storage instance.
	UUID domainstorageprov.FilesystemUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem.
	ProvisionScope domainstorageprov.ProvisionScope
}

// CreateUnitStorageFilesystemAttachmentArg describes a set of arguments for a
// filesystem attachment that should be created alongside a unit's storage in
// the model.
type CreateUnitStorageFilesystemAttachmentArg struct {
	// FilesystemUUID is the unique identifier of the filesystem to be attached.
	FilesystemUUID domainstorageprov.FilesystemUUID

	// NetNodeUUID is the net node of the model entity that filesystem will be
	// attached to.
	NetNodeUUID domainnetwork.NetNodeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem attachment.
	ProvisionScope domainstorageprov.ProvisionScope

	// UUID is the unique identifier to give the filesystem attachment in the
	// model.
	UUID domainstorageprov.FilesystemAttachmentUUID
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
	UUID domainstorageprov.VolumeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created volume.
	ProvisionScope domainstorageprov.ProvisionScope
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
	ProvisionScope domainstorageprov.ProvisionScope

	// VolumeUUID is the unique identifier of the volume to be attached.
	VolumeUUID domainstorageprov.VolumeUUID

	// UUID is the unique identifier to give the volume attachment in the
	// model.
	UUID domainstorageprov.VolumeAttachmentUUID
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

// RegisterUnitStorageArg represents the arguments required for registering a
// unit's storage that has appeared in the model. This struct allows for
// re-using previously created storage for the unit and also provisioning new
// storage as needed.
type RegisterUnitStorageArg struct {
	CreateUnitStorageArg

	// FilesystemProviderIDs defines the provider id value to set for each
	// filesystem. This allows associating new filesystem that are being created
	// with the providers identifier for this storage.
	FilesystemProviderIDs map[domainstorageprov.FilesystemUUID]string

	// VolumeProviderIDs defines the provider id value to set for each volume.
	// This allows associating new volumes that are being created with the
	// providers identifier for this storage.
	VolumeProviderIDs map[domainstorageprov.VolumeUUID]string
}

// StorageInstanceComposition describes the composition of a storage instance
// with in the model. This information is required for attaching existing
// storage in the model to a unit. To be able to properly generate attachments
// this information is required.
type StorageInstanceComposition struct {
	// Filesystem when non-nil describes the filesystem information that is part
	// of the storage composition.
	Filesystem *StorageInstanceCompositionFilesystem

	// StorageName is the name of the storage instance and can be considered to be
	// directly related to the charm storage for which it was provisioned.
	StorageName domainstorage.Name

	// UUID is the unique id of the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// Volume when non nil describes the volume information that is part of the
	// storage composition.
	Volume *StorageInstanceCompositionVolume
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
	ProvisionScope domainstorageprov.ProvisionScope

	// UUID is the unique id of the filesystem that is associated with
	// this storage instance. If the value is nil then no filesystem exists.
	UUID domainstorageprov.FilesystemUUID
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
	ProvisionScope domainstorageprov.ProvisionScope

	// UUID is the unique id of the volume that is associated with this
	// storage instance. If the value is nil then no volume exists.
	UUID domainstorageprov.VolumeUUID
}
