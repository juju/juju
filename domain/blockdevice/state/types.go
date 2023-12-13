// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/domain/blockdevice"
)

// These structs represent the persistent block device entity schema in the database.

type BlockDevice struct {
	ID string `db:"uuid"`

	DeviceName     string `db:"name"`
	Label          string `db:"label"`
	DeviceUUID     string `db:"device_uuid"`
	HardwareId     string `db:"hardware_id"`
	WWN            string `db:"wwn"`
	BusAddress     string `db:"bus_address"`
	SizeMiB        uint64 `db:"size_mib"`
	FilesystemType int    `db:"filesystem_type_id"`
	InUse          bool   `db:"in_use"`
	MountPoint     string `db:"mount_point"`
	SerialId       string `db:"serial_id"`
}

type FilesystemType struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type DeviceLink struct {
	ParentUUID string `db:"block_device_uuid"`
	Name       string `db:"name"`
}

type DeviceMachine struct {
	BlockDeviceUUID string `db:"block_device_uuid"`
}

type BlockDevices []BlockDevice

func (rows BlockDevices) toBlockDevices(deviceLinks []DeviceLink, filesystemTypes []FilesystemType) ([]blockdevice.BlockDevice, error) {
	if n := len(rows); n != len(filesystemTypes) || n != len(deviceLinks) {
		// Should never happen.
		return nil, errors.New("row length mismatch")
	}

	var result []blockdevice.BlockDevice
	recordResult := func(row *BlockDevice, fsType string, deviceLinks []string) {
		result = append(result, blockdevice.BlockDevice{
			DeviceName:     row.DeviceName,
			DeviceLinks:    deviceLinks,
			Label:          row.Label,
			FilesystemType: fsType,
			UUID:           row.DeviceUUID,
			HardwareId:     row.HardwareId,
			WWN:            row.WWN,
			BusAddress:     row.BusAddress,
			SizeMiB:        row.SizeMiB,
			InUse:          row.InUse,
			MountPoint:     row.MountPoint,
			SerialId:       row.SerialId,
		})
	}

	var (
		current *BlockDevice
		fsType  string
		links   []string
	)
	for i, row := range rows {
		if current != nil && row.ID != current.ID {
			recordResult(current, fsType, links)
			links = []string(nil)
		}
		fsType = filesystemTypes[i].Name
		if deviceLinks[i].Name != "" {
			links = append(links, deviceLinks[i].Name)
		}
		rowCopy := row
		current = &rowCopy
	}
	if current != nil {
		recordResult(current, fsType, links)
	}
	return result, nil
}
