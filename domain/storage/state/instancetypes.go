// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"
)

// storageInstanceInfoAttachment represents the attachment information of a
// Storage Instance on to a unit in the model.
type storageInstanceInfoAttachment struct {
	UUID                                  string           `db:"uuid"`
	LifeID                                int              `db:"life_id"`
	StorageFilesystemAttachmentUUID       sql.Null[string] `db:"storage_filesystem_attachment_uuid"`
	StorageFilesystemAttachmentMountPoint sql.Null[string] `db:"storage_filesystem_attachment_mount_point"`
	StorageVolumeAttachmentUUID           sql.Null[string] `db:"storage_volume_attachment_uuid"`
	MachineName                           sql.Null[string] `db:"machine_name"`
	MachineUUID                           sql.Null[string] `db:"machine_uuid"`
	UnitName                              string           `db:"unit_name"`
	UnitUUID                              string           `db:"unit_uuid"`
}

// storageInstanceInfoAttachmentBlockDeviceLink represents the block device link
// information of a Storage Instance attachment on to a unit in the model.
type storageInstanceInfoAttachmentBlockDeviceLink struct {
	StorageAttachmentUUID     string `db:"storage_attachment_uuid",partition:"key"`
	BlockDeviceLinkDeviceName string `db:"block_device_link_device_name"`
}

// storageInstanceInfo represents the information of a Storage Instance in the
// model.
type storageInstanceInfo struct {
	FilesystemStatusID        sql.Null[int]       `db:"storage_filesystem_status_id"`
	FilesystemStatusMessage   sql.Null[string]    `db:"storage_filesystem_status_message"`
	FilesystemStatusUpdatedAt sql.Null[time.Time] `db:"storage_filesystem_status_updated_at"`
	FilesystemUUID            sql.Null[string]    `db:"storage_filesystem_uuid"`
	LifeID                    int                 `db:"life_id"`
	StorageID                 string              `db:"storage_id"`
	StorageKindID             int                 `db:"storage_kind_id"`
	UnitOwnerName             sql.Null[string]    `db:"unit_owner_name"`
	UnitOwnerUUID             sql.Null[string]    `db:"unit_owner_uuid"`
	UUID                      string              `db:"uuid"`
	VolumeStatusID            sql.Null[int]       `db:"storage_volume_status_id"`
	VolumeStatusMessage       sql.Null[string]    `db:"storage_volume_status_message"`
	VolumeStatusUpdatedAt     sql.Null[time.Time] `db:"storage_volume_status_updated_at"`
	VolumeUUID                sql.Null[string]    `db:"storage_volume_uuid"`
}

// Partition returns the Storage Attachment UUID that the block device link
// relates to. Implements the [github.com/juju/juju/internal/iter.Partitionable]
// interface.
func (s storageInstanceInfoAttachmentBlockDeviceLink) Partition() string {
	return s.StorageAttachmentUUID
}
