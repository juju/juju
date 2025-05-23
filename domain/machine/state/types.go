// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// instanceData represents the struct to be inserted into the instance_data
// table.
type instanceData struct {
	MachineUUID          machine.UUID     `db:"machine_uuid"`
	InstanceID           sql.Null[string] `db:"instance_id"`
	DisplayName          sql.Null[string] `db:"display_name"`
	Arch                 *string          `db:"arch"`
	Mem                  *uint64          `db:"mem"`
	RootDisk             *uint64          `db:"root_disk"`
	RootDiskSource       *string          `db:"root_disk_source"`
	CPUCores             *uint64          `db:"cpu_cores"`
	CPUPower             *uint64          `db:"cpu_power"`
	AvailabilityZoneUUID *string          `db:"availability_zone_uuid"`
	VirtType             *string          `db:"virt_type"`
}

// instanceDataResult represents the struct used to retrieve rows when joining
// the machine_cloud_instance table with the availability_zone table.
type instanceDataResult struct {
	MachineUUID      machine.UUID `db:"machine_uuid"`
	InstanceID       string       `db:"instance_id"`
	Arch             *string      `db:"arch"`
	Mem              *uint64      `db:"mem"`
	RootDisk         *uint64      `db:"root_disk"`
	RootDiskSource   *string      `db:"root_disk_source"`
	CPUCores         *uint64      `db:"cpu_cores"`
	CPUPower         *uint64      `db:"cpu_power"`
	AvailabilityZone *string      `db:"availability_zone_name"`
	VirtType         *string      `db:"virt_type"`
}

// instanceTag represents the struct to be inserted into the instance_tag
// table.
type instanceTag struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	Tag         string       `db:"tag"`
}

func tagsFromHardwareCharacteristics(machineUUID machine.UUID, hc *instance.HardwareCharacteristics) []instanceTag {
	if hc == nil || hc.Tags == nil {
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

func (d *instanceDataResult) toHardwareCharacteristics() *instance.HardwareCharacteristics {
	return &instance.HardwareCharacteristics{
		Arch:             d.Arch,
		Mem:              d.Mem,
		RootDisk:         d.RootDisk,
		RootDiskSource:   d.RootDiskSource,
		CpuCores:         d.CPUCores,
		CpuPower:         d.CPUPower,
		AvailabilityZone: d.AvailabilityZone,
		VirtType:         d.VirtType,
	}
}

// machineLife represents the struct to be used for the life_id column within
// the sqlair statements in the machine domain.
type machineLife struct {
	UUID   machine.UUID `db:"uuid"`
	LifeID life.Life    `db:"life_id"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}

// instanceIDAndDisplayName represents the struct to be used for the display_name and ID
// column within the sqlair statements in the machine domain.
type instanceIDAndDisplayName struct {
	ID   string `db:"instance_id"`
	Name string `db:"display_name"`
}

// machineStatus represents the struct to be used for the status.
type machineStatus struct {
	Status  string       `db:"status"`
	Message string       `db:"message"`
	Data    []byte       `db:"data"`
	Updated sql.NullTime `db:"updated_at"`
}

type setStatusInfo struct {
	StatusID int        `db:"status_id"`
	Message  string     `db:"message"`
	Data     []byte     `db:"data"`
	Updated  *time.Time `db:"updated_at"`
}

type setMachineStatus struct {
	StatusID    int          `db:"status_id"`
	Message     string       `db:"message"`
	Data        []byte       `db:"data"`
	Updated     *time.Time   `db:"updated_at"`
	MachineUUID machine.UUID `db:"machine_uuid"`
}

// availabilityZoneName represents the struct to be used for the name column
// within the sqlair statements in the availability_zone table.
type availabilityZoneName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// machineName represents the struct to be used for the name column
// within the sqlair statements in the machine domain.
type machineName struct {
	Name machine.Name `db:"name"`
}

// machineMarkForRemoval represents the struct to be used for the columns of the
// machine_removals table within the sqlair statements in the machine domain.
type machineMarkForRemoval struct {
	UUID machine.UUID `db:"machine_uuid"`
}

// machineUUID represents the struct to be used for the machine_uuid column
// within the sqlair statements in the machine domain.
type machineUUID struct {
	UUID machine.UUID `db:"uuid"`
}

type machineInstanceUUID struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
}

// machineExistsUUID represents the struct to be used for the uuid column
// within the sqlair statements in the machine domain.
type machineExistsUUID struct {
	UUID  machine.UUID `db:"uuid"`
	Count int          `db:"count"`
}

// machineIsController represents the struct to be used for the is_controller column within the sqlair statements in the machine domain.
type machineIsController struct {
	IsController bool `db:"is_controller"`
}

// keepInstance represents the struct to be used for the keep_instance column
// within the sqlair statements in the machine domain.
type keepInstance struct {
	KeepInstance bool `db:"keep_instance"`
}

// machineParent represents the struct to be used for the columns of the
// machine_parent table within the sqlair statements in the machine domain.
type machineParent struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	ParentUUID  machine.UUID `db:"parent_uuid"`
}

// uuidSliceTransform is a function that is used to transform a slice of
// machineUUID into a slice of string.
func (s machineMarkForRemoval) uuidSliceTransform() machine.UUID {
	return s.UUID
}

// nameSliceTransform is a function that is used to transform a slice of
// machineName into a slice of machine.Name.
func (s machineName) nameSliceTransform() machine.Name {
	return s.Name
}

func decodeMachineStatus(s string) (domainmachine.MachineStatusType, error) {
	var result domainmachine.MachineStatusType
	switch s {
	case "error":
		result = domainmachine.MachineStatusError
	case "started":
		result = domainmachine.MachineStatusStarted
	case "pending":
		result = domainmachine.MachineStatusPending
	case "stopped":
		result = domainmachine.MachineStatusStopped
	case "down":
		result = domainmachine.MachineStatusDown
	case "":
		result = domainmachine.MachineStatusUnknown
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func encodeMachineStatus(s domainmachine.MachineStatusType) (int, error) {
	var result int
	switch s {
	case domainmachine.MachineStatusError:
		result = 0
	case domainmachine.MachineStatusStarted:
		result = 1
	case domainmachine.MachineStatusPending:
		result = 2
	case domainmachine.MachineStatusStopped:
		result = 3
	case domainmachine.MachineStatusDown:
		result = 4
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func decodeCloudInstanceStatus(s string) (domainmachine.InstanceStatusType, error) {
	var result domainmachine.InstanceStatusType
	switch s {
	case "unknown", "":
		result = domainmachine.InstanceStatusUnset
	case "pending":
		result = domainmachine.InstanceStatusPending
	case "allocating":
		result = domainmachine.InstanceStatusAllocating
	case "running":
		result = domainmachine.InstanceStatusRunning
	case "provisioning error":
		result = domainmachine.InstanceStatusProvisioningError
	default:
		return 0, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func encodeCloudInstanceStatus(s domainmachine.InstanceStatusType) (int, error) {
	var result int
	switch s {
	case domainmachine.InstanceStatusUnset:
		result = 0
	case domainmachine.InstanceStatusPending:
		result = 1
	case domainmachine.InstanceStatusAllocating:
		result = 2
	case domainmachine.InstanceStatusRunning:
		result = 3
	case domainmachine.InstanceStatusProvisioningError:
		result = 4
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// createMachineArgs represents the struct to be used for the input parameters
// of the createMachine state method in the machine domain.
type createMachineArgs struct {
	name        machine.Name
	machineUUID machine.UUID
	netNodeUUID string
	parentName  machine.Name
}

// lxdProfile represents the struct to be used for the sqlair statements on the
// lxd_profile table.
type lxdProfile struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	Name        string       `db:"name"`
	Index       int          `db:"array_index"`
}
