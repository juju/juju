// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// CreateStorageInstanceWithExistingFilesystem is used to create a storage
// instance with a filesystem that is already provisioned.
type CreateStorageInstanceWithExistingFilesystem struct {
	// Name is the name of the storage.
	Name domainstorage.Name

	// RequestedSizeMiB defines the requested size of this storage instance in
	// MiB. What ends up being allocated for the storage instance will be at
	// least this value.
	RequestedSizeMiB uint64

	// StoragePoolUUID is the pool for which this storage instance is to be
	// provisioned from.
	StoragePoolUUID domainstorage.StoragePoolUUID

	// UUID is the unique identifier to associate with the storage instance.
	UUID domainstorage.StorageInstanceUUID

	// FilesystemUUID describes the unique identifier of the filesystem to
	// create alongside the storage instance.
	FilesystemUUID domainstorage.FilesystemUUID

	// FilesystemProvisionScope describes the provision scope to assign to the
	// newly created filesystem.
	FilesystemProvisionScope domainstorageprov.ProvisionScope

	// FilesystemSize is the size of the provisioned filesystem.
	FilesystemSize uint64

	// FilesystemProviderID is provider's ID for the provisioned filesystem.
	FilesystemProviderID string

	// FilesystemStatusID is the value to set the storage filesystem status to.
	FilesystemStatusID int

	// FilesystemStatusMessage is the message to set the storage filesystem
	// status to.
	FilesystemStatusMessage string

	// FilesystemStatusUpdatedAt is the time at which the storage filesystem
	// status updated at should reflect.
	FilesystemStatusUpdatedAt time.Time
}

// CreateStorageInstanceWithExistingVolumeBackedFilesystem is used to create a
// storage instance with a volume backed filesystem that is already provisioned.
type CreateStorageInstanceWithExistingVolumeBackedFilesystem struct {
	CreateStorageInstanceWithExistingFilesystem

	// VolumeUUID describes the unique identifier of the volume to create
	// alongside the storage instance.
	VolumeUUID domainstorage.VolumeUUID

	// VolumeProvisionScope describes the provision scope to assign to the newly
	// created volume.
	VolumeProvisionScope domainstorageprov.ProvisionScope

	// VolumeSize is the size of the provisioned volume.
	VolumeSize uint64

	// VolumeProviderID is provider's ID for the provisioned volume.
	VolumeProviderID string

	// VolumeHardwareID is set by the storage provider to help matching with a
	// block device.
	VolumeHardwareID string

	// VolumeWWN is set by the storage provider to help matching with a block
	// device.
	VolumeWWN string

	// VolumePersistent is true if the volume is persistent.
	VolumePersistent bool

	// VolumeStatusID is the value to set the storage volume status to.
	VolumeStatusID int

	// VolumeStatusMessage is the message to set the storage volume status to.
	VolumeStatusMessage string

	// VolumeStatusUpdatedAt is the time at which the storage volume status
	// updated at should reflect.
	VolumeStatusUpdatedAt time.Time
}
