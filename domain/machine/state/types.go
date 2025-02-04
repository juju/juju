// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// instanceData represents the struct to be inserted into the instance_data
// table.
type instanceData struct {
	MachineUUID          string  `db:"machine_uuid"`
	InstanceID           string  `db:"instance_id"`
	DisplayName          string  `db:"display_name"`
	Arch                 *string `db:"arch"`
	Mem                  *uint64 `db:"mem"`
	RootDisk             *uint64 `db:"root_disk"`
	RootDiskSource       *string `db:"root_disk_source"`
	CPUCores             *uint64 `db:"cpu_cores"`
	CPUPower             *uint64 `db:"cpu_power"`
	AvailabilityZoneUUID *string `db:"availability_zone_uuid"`
	VirtType             *string `db:"virt_type"`
}

// instanceDataResult represents the struct used to retrieve rows when joining
// the machine_cloud_instance table with the availability_zone table.
type instanceDataResult struct {
	MachineUUID      string  `db:"machine_uuid"`
	InstanceID       string  `db:"instance_id"`
	Arch             *string `db:"arch"`
	Mem              *uint64 `db:"mem"`
	RootDisk         *uint64 `db:"root_disk"`
	RootDiskSource   *string `db:"root_disk_source"`
	CPUCores         *uint64 `db:"cpu_cores"`
	CPUPower         *uint64 `db:"cpu_power"`
	AvailabilityZone *string `db:"availability_zone_name"`
	VirtType         *string `db:"virt_type"`
}

// instanceTag represents the struct to be inserted into the instance_tag
// table.
type instanceTag struct {
	MachineUUID string `db:"machine_uuid"`
	Tag         string `db:"tag"`
}

func tagsFromHardwareCharacteristics(machineUUID string, hc *instance.HardwareCharacteristics) []instanceTag {
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
	UUID   string    `db:"uuid"`
	LifeID life.Life `db:"life_id"`
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

// setMachineStatus represents the struct to be used for the columns of the
// machine_status table within the sqlair statements in the machine domain.
type setMachineStatus struct {
	MachineUUID string     `db:"machine_uuid"`
	StatusID    int        `db:"status_id"`
	Message     string     `db:"message"`
	Data        []byte     `db:"data"`
	Updated     *time.Time `db:"updated_at"`
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
	UUID string `db:"machine_uuid"`
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

// keepInstance represents the struct to be used for the keep_instance column
// within the sqlair statements in the machine domain.
type keepInstance struct {
	KeepInstance bool `db:"keep_instance"`
}

// machineParent represents the struct to be used for the columns of the
// machine_parent table within the sqlair statements in the machine domain.
type machineParent struct {
	MachineUUID string `db:"machine_uuid"`
	ParentUUID  string `db:"parent_uuid"`
}

// uuidSliceTransform is a function that is used to transform a slice of
// machineUUID into a slice of string.
func (s machineMarkForRemoval) uuidSliceTransform() string {
	return s.UUID
}

// nameSliceTransform is a function that is used to transform a slice of
// machineName into a slice of machine.Name.
func (s machineName) nameSliceTransform() machine.Name {
	return s.Name
}

func decodeMachineStatus(s string) (status.Status, error) {
	var result status.Status
	switch s {
	case "error":
		result = status.Error
	case "started":
		result = status.Started
	case "pending":
		result = status.Pending
	case "stopped":
		result = status.Stopped
	case "down":
		result = status.Down
	case "":
		result = status.Unknown
	default:
		return status.Unknown, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func encodeMachineStatus(s status.Status) (int, error) {
	var result int
	switch s {
	case status.Error:
		result = 0
	case status.Started:
		result = 1
	case status.Pending:
		result = 2
	case status.Stopped:
		result = 3
	case status.Down:
		result = 4
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func decodeCloudInstanceStatus(s string) (status.Status, error) {
	var result status.Status
	switch s {
	case "unknown", "":
		result = status.Empty
	case "allocating":
		result = status.Allocating
	case "running":
		result = status.Running
	case "provisioning error":
		result = status.ProvisioningError
	default:
		return status.Unknown, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

func encodeCloudInstanceStatus(s status.Status) (int, error) {
	var result int
	switch s {
	case status.Empty:
		result = 0
	case status.Allocating:
		result = 1
	case status.Running:
		result = 2
	case status.ProvisioningError:
		result = 3
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// createMachineArgs represents the struct to be used for the input parameters
// of the createMachine state method in the machine domain.
type createMachineArgs struct {
	name        machine.Name
	machineUUID string
	netNodeUUID string
	parentName  machine.Name
}

// lxdProfile represents the struct to be used for the sqlair statements on the
// lxd_profile table.
type lxdProfile struct {
	MachineUUID string `db:"machine_uuid"`
	Name        string `db:"name"`
	Index       int    `db:"array_index"`
}
