// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/domain/application/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// CreateApplicationStorageDirectiveArg defines an individual storage directive to be
// associated with an application.
type CreateApplicationStorageDirectiveArg = CreateStorageDirectiveArg

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

// CreateStorageAttachmentArg describes the arguments required for creating a
// storage attachment.
type CreateStorageAttachmentArg struct {
	// UUID is the unique identifier to associate with the storage attachment.
	UUID domainstorageprov.StorageAttachmentUUID

	// StorageInstanceUUID is the unique identifier of the storage instance
	// to attach to the unit.
	StorageInstanceUUID domainstorage.StorageInstanceUUID
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
	StorageToAttach []CreateStorageAttachmentArg

	// StorageToOwn defines the storage instances that should be owned by the
	// unit.
	StorageToOwn []domainstorage.StorageInstanceUUID
}

// DefaultStorageProvisioners defines the set of default storage provisioners
// for each type of storage that can be provisioned in a model. If a storage
// type has no default provisioner set then a default does not exist for the
// model.
type DefaultStorageProvisioners struct {
	// BlockdevicePoolUUID describes the storage pool uuid that should be used
	// when provisioning new block device storage in the model.
	BlockdevicePoolUUID *domainstorage.StoragePoolUUID

	// FilesystemPoolUUID describes the storage pool uuid that should be used
	// when provisioning new filesystem storage in the model.
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
	// with a unit with the information we already have from the provider.
	FilesystemProviderIDs map[domainstorageprov.FilesystemUUID]string
}

// StorageDirective defines a storage directive that already exists for either
// an application or unit.
type StorageDirective struct {
	// CharmMetadataName is the metadata name of the charm the directive exists for.
	CharmMetadataName string

	// Count represents the number of storage instances that should be made for
	// this directive. This value should be the desired count but not the limit.
	// For the maximum supported limit see [StorageDirective.MaxCount].
	Count uint32

	// CharmStorageType represents the storage type of the charm that the
	// directive relates to.
	CharmStorageType charm.StorageType

	// MaxCount represents the maximum number of storage instances that can be
	// made for this directive.
	MaxCount uint32

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive.
	PoolUUID domainstorage.StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

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
