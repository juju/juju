// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/errors"
)

// These structs represent the persistent block device entity schema in the database.

type BlockDevice struct {
	ID          string `db:"uuid"`
	MachineUUID string `db:"machine_uuid"`

	DeviceName     string `db:"name"`
	Label          string `db:"label,omitempty"`
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

type BlockDeviceMachine struct {
	MachineId string `db:"name"`
}

type BlockDevices []BlockDevice

func (rows BlockDevices) toBlockDevicesAndMachines(deviceLinks []DeviceLink, filesystemTypes []FilesystemType, machines []BlockDeviceMachine) ([]blockdevice.BlockDevice, []string, error) {
	if n := len(rows); n != len(filesystemTypes) || n != len(deviceLinks) || (machines != nil && n != len(machines)) {
		// Should never happen.
		return nil, nil, errors.New("row length mismatch composing block device results")
	}

	var (
		resultBlockDevices []blockdevice.BlockDevice
		resultMachines     []string
	)
	recordResult := func(row *BlockDevice, fsType string, deviceLinks []string, machineId string) {
		resultBlockDevices = append(resultBlockDevices, blockdevice.BlockDevice{
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
		resultMachines = append(resultMachines, machineId)
	}

	var (
		current   *BlockDevice
		fsType    string
		links     []string
		machineId string
	)
	for i, row := range rows {
		if current != nil && row.ID != current.ID {
			recordResult(current, fsType, links, machineId)
			links = []string(nil)
		}
		fsType = filesystemTypes[i].Name
		if machines != nil {
			machineId = machines[i].MachineId
		}
		if deviceLinks[i].Name != "" {
			links = append(links, deviceLinks[i].Name)
		}
		rowCopy := row
		current = &rowCopy
	}
	if current != nil {
		recordResult(current, fsType, links, machineId)
	}
	return resultBlockDevices, resultMachines, nil
}
