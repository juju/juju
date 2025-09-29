// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
)

type dbStorageInstanceInfo struct {
	ID            string         `db:"storage_id"`
	OwnerUnitName sql.NullString `db:"owner_unit_name"`
	KindID        int            `db:"storage_kind_id"`
	LifeID        int            `db:"life_id"`
	Persistent    bool           `db:"persistent"`
}

type dbVolumeAttachmentInfo struct {
	StorageID string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	HardwareID string `db:"hardware_id"`
	WWN        string `db:"wwn"`

	BlockDeviceName string `db:"block_device_name"`
	BlockDeviceLink string `db:"block_device_link"`
}

type dbFilesystemAttachmentInfo struct {
	StorageID string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	MountPoint string `db:"mount_point"`
}

// StorageInstanceInfo describes information about a storage instance.
type StorageInstanceInfo struct {
	ID         string
	Owner      *unit.Name
	Kind       storage.StorageKind
	Life       life.Life
	Persistent bool

	VolumeInfo     *VolumeInfo
	FilesystemInfo *FilesystemInfo
}

// VolumeInfo describes information about a volume with its attachments.
type VolumeInfo struct {
	Status      status.StatusInfo[status.StorageVolumeStatusType]
	Attachments []VolumeAttachmentInfo
}

// FilesystemInfo describes information about a filesystem with its attachments.
type FilesystemInfo struct {
	Status      status.StatusInfo[status.StorageFilesystemStatusType]
	Attachments []FilesystemAttachmentInfo
}

// VolumeAttachmentInfo describes information about a volume attachment.
type VolumeAttachmentInfo struct {
	AttachmentInfo
	HardwareID      string
	WWN             string
	BlockDeviceName string
	BlockDeviceLink string
}

// FilesystemAttachmentInfo describes information about a filesystem attachment.
type FilesystemAttachmentInfo struct {
	AttachmentInfo
	MountPoint string
}

// AttachmentInfo describes information about a storage attachment.
type AttachmentInfo struct {
	Life    life.Life
	Unit    unit.Name
	Machine *machine.Name
}
