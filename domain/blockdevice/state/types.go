// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/domain/life"
)

type entityUUID struct {
	UUID string `db:"uuid"`
}

type blockDevice struct {
	UUID        string `db:"uuid"`
	MachineUUID string `db:"machine_uuid"`

	Name sql.Null[string] `db:"name"`

	HardwareId string `db:"hardware_id"`
	WWN        string `db:"wwn"`
	BusAddress string `db:"bus_address"`
	SerialId   string `db:"serial_id"`

	SizeMiB         uint64 `db:"size_mib"`
	FilesystemLabel string `db:"filesystem_label"`
	FilesystemUUID  string `db:"filesystem_uuid"`
	FilesystemType  string `db:"filesystem_type"`
	InUse           bool   `db:"in_use"`
	MountPoint      string `db:"mount_point"`
}

type deviceLink struct {
	BlockDeviceUUID string `db:"block_device_uuid"`
	MachineUUID     string `db:"machine_uuid"`
	Name            string `db:"name"`
}

type entityLife struct {
	UUID string    `db:"uuid"`
	Life life.Life `db:"life_id"`
}

type entityName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}
