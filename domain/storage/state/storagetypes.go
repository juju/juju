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
)

type dbStorageInstanceDetails struct {
	ID            string         `db:"storage_id"`
	OwnerUnitName sql.NullString `db:"owner_unit_name"`
	KindID        int            `db:"storage_kind_id"`
	LifeID        int            `db:"life_id"`
	Persistent    bool           `db:"persistent"`
}

type dbVolumeAttachmentDetails struct {
	StorageID string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	BlockDeviceUUID string `db:"block_device_uuid"`
}

type dbFilesystemAttachmentDetails struct {
	StorageID string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	MountPoint string `db:"mount_point"`
}

// VolumeDetails describes information about a volume with its attachments.
type VolumeDetails struct {
	StorageID   string
	Status      status.StatusInfo[status.StorageVolumeStatusType]
	Attachments []VolumeAttachmentDetails
}

// FilesystemDetails describes information about a filesystem with its attachments.
type FilesystemDetails struct {
	StorageID   string
	Status      status.StatusInfo[status.StorageFilesystemStatusType]
	Attachments []FilesystemAttachmentDetails
}

// VolumeAttachmentDetails describes information about a volume attachment.
type VolumeAttachmentDetails struct {
	AttachmentDetails
	BlockDeviceUUID string
}

// FilesystemAttachmentDetails describes information about a filesystem attachment.
type FilesystemAttachmentDetails struct {
	AttachmentDetails
	MountPoint string
}

// AttachmentDetails describes information about a storage attachment.
type AttachmentDetails struct {
	Life    life.Life
	Unit    unit.Name
	Machine *machine.Name
}
