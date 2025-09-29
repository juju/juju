// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// ModelStoragePools provides the default storage pools that have been set
// within the model. If a value is nil then no default exists.
type ModelStoragePools struct {
	// BlockDevicePoolUUID provides the storage pool uuid to use for new block
	// storage.
	BlockDevicePoolUUID *domainstorage.StoragePoolUUID

	// FilesystemPoolUUID provides the storage pool uuid to use for
	// filesystem storage.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID
)

// StorageInstanceComposition describes the composition of a storage instance
// with in the model. This information is required for attaching existing
// storage in the model to a unit. To be able to properly generate attachments
// this information is required.
type StorageInstanceComposition struct {
	// Filesystem when non nil describes the filesystem information that is part
	// of the storage composition.
	Filesystem *StorageInstanceCompositionFilesystem

	// StorageName is the name of the storage instance and can be considered to be
	// directly related to the chamr storage for which it was provisioned.
	StorageName string

	// UUID is the unique id of the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// Volume when non nil describes the volume information that is part of the
	// storage composition.
	Volume *StorageInstanceCompositionVolume
}

// StorageInstanceCompositionFilesystem describes the filesystem information
// that is part of a [StorageInstanceComposition].
type StorageInstanceCompositionFilesystem struct {
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
	// ProvisionScope is the provision scope of the volume that is
	// attached to this storage instance. This value is only considered valid
	// when [StorageInstanceComposition.VolumeUUID] is not nil.
	ProvisionScope domainstorageprov.ProvisionScope

	// UUID is the unique id of the volume that is associated with this
	// storage instance. If the value is nil then no volume exists.
	UUID domainstorageprov.VolumeUUID
}
