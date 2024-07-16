// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
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
	if hc.Tags == nil {
		return nil
	}
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
	UUID   string    `db:"uuid"`
	LifeID life.Life `db:"life_id"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}

// machineInstanceStatus represents the struct to be used for the status columns
// of the machine_status and the machine_cloud_instance_status tables) within
// the sqlair statements in the machine domain.
type machineInstanceStatus struct {
	Status  int        `db:"status"`
	Message string     `db:"message"`
	Updated *time.Time `db:"updated_at"`
}

// machineInstanceStatusData represents the struct to be used for the status
// data columns of the machine_status_data and the
// machine_cloud_instance_status_data tables within the sqlair statements in the
// machine domain.
type machineInstanceStatusData struct {
	Key  string `db:"key"`
	Data string `db:"data"`
}

// machineName represents the struct to be used for the name column
// within the sqlair statements in the machine domain.
type machineName struct {
	Name machine.Name `db:"name"`
}

// machineUUID represents the struct to be used for the machine_uuid column
// within the sqlair statements in the machine domain.
type machineUUID struct {
	UUID string `db:"uuid"`
}

// machineIsController represents the struct to be used for the is_controller column within the sqlair statements in the machine domain.
type machineIsController struct {
	IsController bool `db:"is_controller"`
}

// toCoreMachineStatusValue converts an internal status used by machines (per
// the machine_status_value table) into a core type status.Status.
func (s *machineInstanceStatus) toCoreMachineStatusValue() status.Status {
	var out status.Status
	switch s.Status {
	case 0:
		out = status.Error
	case 1:
		out = status.Started
	case 2:
		out = status.Pending
	case 3:
		out = status.Stopped
	case 4:
		out = status.Down
	}
	return out
}

// fromCoreMachineStatusValue converts a status.Status to an internal status
// used by machines (per the machine_status_value table).
func fromCoreMachineStatusValue(s status.Status) int {
	var internalStatus int
	switch s {
	case status.Error:
		internalStatus = 0
	case status.Started:
		internalStatus = 1
	case status.Pending:
		internalStatus = 2
	case status.Stopped:
		internalStatus = 3
	case status.Down:
		internalStatus = 4
	}
	return internalStatus
}

// toCoreInstanceStatusValue converts an internal status used by machine cloud
// instances (per the instance_status_values table) into a core type
// status.Status.
func (s *machineInstanceStatus) toCoreInstanceStatusValue() status.Status {
	var out status.Status
	switch s.Status {
	case 0:
		out = status.Empty
	case 1:
		out = status.Allocating
	case 2:
		out = status.Running
	case 3:
		out = status.ProvisioningError
	}
	return out
}

// fromCoreInstanceStatusValue converts a status.Status to an internal status
// used by machine cloud instances (per the instance_status_value table).
func fromCoreInstanceStatusValue(s status.Status) int {
	var internalStatus int
	switch s {
	case status.Empty:
		internalStatus = 0
	case status.Allocating:
		internalStatus = 1
	case status.Running:
		internalStatus = 2
	case status.ProvisioningError:
		internalStatus = 3
	}
	return internalStatus
}
