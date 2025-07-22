// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// CreateApplicationStorageDirectiveArg defines an individual storage directive to be
// associated with an application.
type CreateApplicationStorageDirectiveArg = CreateStorageDirectiveArg

// CreateUnitStorageDirectiveArg describes the arguments required for making storage
// directives on a unit.
type CreateUnitStorageDirectiveArg = CreateStorageDirectiveArg

// CreateUnitStorageInstanceArg describes a set of arguments that create a new
// storage instance on behalf of a unit.
type CreateUnitStorageInstanceArg struct {
	// Name is the name of the storage and must correspond to the storage name
	// defined in the charm the unit is running.
	Name domainstorage.Name

	// UUID is the unique identifier to associate with the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// FilesystemUUID describes the unique identifier of the filesystem to
	// create alongside the storage instance. If this value is nil no file
	// system will be created.
	FilesystemUUID *domainstorageprov.FilesystemUUID

	// VolumeUUID describes the unique identifier of the volume to
	// create alongside the storage instance. If this value is nil no volume
	// will be created.
	VolumeUUID *domainstorageprov.VolumeUUID
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
	StorageToAttach []domainstorage.StorageInstanceUUID

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
	// when provisioning new block device storage in the model. If this value is
	// set then [defaultStorageProvisioners.BlockdeviceProviderType] will not be
	// set.
	BlockdevicePoolUUID *domainstorage.StoragePoolUUID

	// BlockdeviceProviderType describes the storage provider type that should
	// be used when provisioning new block device storage in the model. If this
	// value is set then [defaultStorageProvisioners.BlockdevicePoolUUID] will
	// not be set.
	BlockdeviceProviderType *string

	// FilesystemPoolUUID describes the storage pool uuid that should be used
	// when provisioning new filesystem storage in the model. If this value is
	// set then [defaultStorageProvisioners.FilesystemProviderType] will not be
	// set.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID

	// FilesystemProviderType describes the storage provider type that should
	// be used when provisioning new filesystem storage in the model. If this
	// value is set then [defaultStorageProvisioners.FilesystemPoolUUID] will
	// not be set.
	FilesystemProviderType *string
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
	// Count represents the number of storage instances that should be made for
	// this directive.
	Count uint32

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive. This is
	// an optional value and if not set it is expected that
	// [ApplicationStorageDirectiveArg.ProviderType] is set.
	PoolUUID *domainstorage.StoragePoolUUID

	// ProviderType defines the storage provider type to use for the directive.
	// This is an optional value and if not set it is expected that
	// [ApplicationStorageDirectiveArg.PoolUUID] is set.
	ProviderType *string

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
	PoolUUID *domainstorage.StoragePoolUUID

	// ProviderType defines the storage provider type to use for the directive.
	// This is an optional value and if not set it is expected that
	// [ApplicationStorageDirectiveArg.PoolUUID] is set.
	ProviderType *string

	// Size defines the size of the storage directive in MiB.
	Size uint64
}
