// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// CreateUnitStorageInstanceArg describes a set of arguments that create a new
// storage instance on behalf of a unit.
type CreateUnitStorageInstanceArg struct {
	// Name is the name of the storage and must correspond to the storage name
	// defined in the charm the unit is running.
	Name string

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
	StorageDirectives []UnitStorageDirectiveArg

	// StorageInstances defines the new storage instances that must be created
	// for the unit.
	StorageInstances []CreateUnitStorageInstanceArg

	// StorageToAttach defines the storage instances that should be attached to
	// the unit. New storage instances defined in
	// [CreateUnitStorageArg.StorageInstances] are not automatically attached to
	// the unit and should be included in this list.
	StorageToAttach []domainstorage.StorageInstanceUUID
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
