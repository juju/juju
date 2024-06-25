// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/domain/life"
)

// instanceData represents the struct to be inserted into the instance_data
// table.
type instanceData struct {
	MachineUUID          string  `db:"machine_uuid"`
	InstanceID           string  `db:"instance_id"`
	Arch                 *string `db:"arch"`
	Mem                  *uint64 `db:"mem"`
	RootDisk             *uint64 `db:"root_disk"`
	RootDiskSource       *string `db:"root_disk_source"`
	CPUCores             *uint64 `db:"cpu_cores"`
	CPUPower             *uint64 `db:"cpu_power"`
	AvailabilityZoneUUID *string `db:"availability_zone_uuid"`
	VirtType             *string `db:"virt_type"`
}

// instanceTag represents the struct to be inserted into the instance_tag
// table.
type instanceTag struct {
	MachineUUID string `db:"machine_uuid"`
	Tag         string `db:"tag"`
}

func tagsFromHardwareCharacteristics(machineUUID string, hc *instance.HardwareCharacteristics) []instanceTag {
	res := make([]instanceTag, len(*hc.Tags))
	for i, tag := range *hc.Tags {
		res[i] = instanceTag{
			MachineUUID: machineUUID,
			Tag:         tag,
		}
	}
	return res
}

func (d *instanceData) toHardwareCharacteristics() *instance.HardwareCharacteristics {
	return &instance.HardwareCharacteristics{
		Arch:             d.Arch,
		Mem:              d.Mem,
		RootDisk:         d.RootDisk,
		RootDiskSource:   d.RootDiskSource,
		CpuCores:         d.CPUCores,
		CpuPower:         d.CPUPower,
		AvailabilityZone: d.AvailabilityZoneUUID,
		VirtType:         d.VirtType,
	}
}

// machineLife represents the struct to be used for the life_id column within
// the sqlair statements in the machine domain.
type machineLife struct {
	ID life.Life `db:"life_id"`
}
