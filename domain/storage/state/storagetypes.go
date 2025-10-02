// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"
)

type dbStorageInstanceDetails struct {
	UUID          string         `db:"uuid"`
	ID            string         `db:"storage_id"`
	OwnerUnitName sql.NullString `db:"owner_unit_name"`
	KindID        int            `db:"storage_kind_id"`
	LifeID        int            `db:"life_id"`
	Persistent    bool           `db:"persistent"`
}

type dbVolumeAttachmentDetails struct {
	StorageInstanceUUID string `db:"uuid"`
	StorageID           string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	BlockDeviceUUID string `db:"block_device_uuid"`
}

type dbFilesystemAttachmentDetails struct {
	StorageInstanceUUID string `db:"uuid"`
	StorageID           string `db:"storage_id"`

	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	UpdatedAt *time.Time `db:"updated_at"`

	LifeID      int            `db:"life_id"`
	UnitName    sql.NullString `db:"unit_name"`
	MachineName sql.NullString `db:"machine_name"`

	MountPoint string `db:"mount_point"`
}
