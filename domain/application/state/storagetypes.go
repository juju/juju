// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"
)

// insertStorageFilesystem represents the set of values required for inserting a
// new storage filesystem into the model.
type insertStorageFilesystem struct {
	FilesystemID     string `db:"filesystem_id"`
	LifeID           int    `db:"life_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
	UUID             string `db:"uuid"`
}

// insertStorageFilesystemAttachment represents the set of values required for
// creating a new filesystem attachment in the model.
type insertStorageFilesystemAttachment struct {
	LifeID                int    `db:"life_id"`
	NetNodeUUID           string `db:"net_node_uuid"`
	ProvisionScopeID      int    `db:"provision_scope_id"`
	StorageFilesystemUUID string `db:"storage_filesystem_uuid"`
	UUID                  string `db:"uuid"`
}

// insertStorageFilesystemInstance represents the set of values required for
// assocating a storage instance and filesystem that already exist in the model
// together.
type insertStorageFilesystemInstance struct {
	StorageFilesystemUUUID string `db:"storage_filesystem_uuid"`
	StorageInstanceUUID    string `db:"storage_instance_uuid"`
}

// insertStorageFilesystemStatus represents the set of values required for
// creating a new status value on a filesystem.
type insertStorageFilesystemStatus struct {
	FilesystemUUID string    `db:"filesystem_uuid"`
	StatusID       int       `db:"status_id"`
	UpdateAt       time.Time `db:"updated_at"`
}

// insertStorageInstance represents the set of values required for inserting a
// new storage instance into the model.
type insertStorageInstance struct {
	CharmName       string `db:"charm_name"`
	LifeID          int    `db:"life_id"`
	RequestSizeMiB  uint64 `db:"requested_size_mib"`
	StorageID       string `db:"storage_id"`
	StorageKindID   int    `db:"storage_kind_id"`
	StorageName     string `db:"storage_name"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
	UUID            string `db:"uuid"`
}

// insertStorageInstanceAttachment represents the set of values required for
// inserting a new attachment for a storage instance into the model.
type insertStorageInstanceAttachment struct {
	LifeID              int    `db:"life_id"`
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
	UUID                string `db:"uuid"`
}

// insertStorageVolume represents the set of values required for inserting a
// new storage volume into the model.
type insertStorageVolume struct {
	LifeID           int    `db:"life_id"`
	UUID             string `db:"uuid"`
	VolumeID         string `db:"volume_id"`
	ProvisionScopeID int    `db:"provision_scope_id"`
}

// insertStorageVolumeAttachment represents the set of values required for
// creating a new filesystem attachment in the model.
type insertStorageVolumeAttachment struct {
	LifeID            int    `db:"life_id"`
	NetNodeUUID       string `db:"net_node_uuid"`
	ProvisionScopeID  int    `db:"provision_scope_id"`
	StorageVolumeUUID string `db:"storage_volume_uuid"`
	UUID              string `db:"uuid"`
}

// insertStorageVolumeInstance represents the set of values required for
// associating a storage instance and volume that already exist in the model
// together.
type insertStorageVolumeInstance struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	StorageVolumeUUID   string `db:"storage_volume_uuid"`
}

// insertStorageVolumeStatus represents the set of values required for
// creating a new status value on a volume.
type insertStorageVolumeStatus struct {
	VolumeUUID string    `db:"volume_uuid"`
	StatusID   int       `db:"status_id"`
	UpdateAt   time.Time `db:"updated_at"`
}

// storageProviderIDs  represents a list of provider ids that have been given to
// either a volume or filesystem in the model.
type storageProviderIDs []string

// storageDirective represents either a storage directive from a unit in the
// model or an application.
type storageDirective struct {
	CharmMetadataName string `db:"charm_metadata_name"`
	CharmStorageKind  string `db:"charm_storage_kind"`
	Count             uint32 `db:"count"`
	CountMax          uint32 `db:"count_max"`
	SizeMiB           uint64 `db:"size_mib"`
	StorageName       string `db:"storage_name"`
	StoragePoolUUID   string `db:"storage_pool_uuid"`
}

// storageInstanceComposition is used to get the composition of a storage
// instance within the model.
type storageInstanceComposition struct {
	FilesystemProvisionScope sql.Null[int]    `db:"filesystem_provision_scope"`
	FilesystemUUID           sql.Null[string] `db:"filesystem_uuid"`
	StorageName              string           `db:"storage_name"`
	UUID                     string           `db:"uuid"`
	VolumeProvisionScope     sql.Null[int]    `db:"volume_provision_scope"`
	VolumeUUID               sql.Null[string] `db:"volume_uuid"`
}

// storageModelConfigKeys is used to get model config to select the storage pool
// or storage provider type.
type storageModelConfigKeys struct {
	BlockDeviceKey string `db:"blockdevice_key"`
	FilesystemKey  string `db:"filesystem_key"`
}

type modelStoragePools struct {
	StorageKindID   int    `db:"storage_kind_id"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
}
