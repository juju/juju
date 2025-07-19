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
	FilesystemID string `db:"filesystem_id"`
	LifeID       int    `db:"life_id"`
	UUID         string `db:"uuid"`
}

// insertStorageFilesystemAttachment represents the set of values required for
// creating a new filesystem attachment in the model.
type insertStorageFilesystemAttachment struct {
	LifeID                int    `db:"life_id"`
	NetNodeUUID           string `db:"net_node_uuid"`
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
	CharmUUID       string           `db:"charm_uuid"`
	LifeID          int              `db:"life_id"`
	RequestSizeMiB  uint64           `db:"requested_size_mib"`
	StorageID       string           `db:"storage_id"`
	StorageName     string           `db:"storage_name"`
	StoragePoolUUID sql.Null[string] `db:"storage_pool_uuid"`
	StorageType     sql.Null[string] `db:"storage_type"`
	UUID            string           `db:"uuid"`
}

// insertStorageInstanceAttachment represents the set of values required for
// inserting a new attachment for a storage instance into the model.
type insertStorageInstanceAttachment struct {
	LifeID              int    `db:"life_id"`
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
}

// insertStorageVolume represents the set of values required for inserting a
// new storage volume into the model.
type insertStorageVolume struct {
	LifeID   int    `db:"life_id"`
	UUID     string `db:"uuid"`
	VolumeID string `db:"volume_id"`
}

// insertStorageVolumeAttachment represents the set of values required for
// creating a new filesystem attachment in the model.
type insertStorageVolumeAttachment struct {
	LifeID            int    `db:"life_id"`
	NetNodeUUID       string `db:"net_node_uuid"`
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

// storageFilesystemUUIDRef is a database type for selecting a foreign key
// reference to a storage filesystem uuid.
type storageFilesystemUUIDRef struct {
	UUID string `db:"storage_filesystem_uuid"`
}

// storageVolumeUUIDRef is a database type for selecting a foreign key reference
// to a storage volume uuid.
type storageVolumeUUIDRef struct {
	UUID string `db:"storage_volume_uuid"`
}
