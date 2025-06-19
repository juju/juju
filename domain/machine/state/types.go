// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/life"
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

type availabilityZoneName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type machineName struct {
	Name machine.Name `db:"name"`
}

type machineMarkForRemoval struct {
	UUID machine.UUID `db:"machine_uuid"`
}

type machineUUID struct {
	UUID machine.UUID `db:"uuid"`
}

type machineInstanceUUID struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
}

type machineExistsUUID struct {
	UUID  machine.UUID `db:"uuid"`
	Count int          `db:"count"`
}

type count struct {
	Count int64 `db:"count"`
}

type keepInstance struct {
	KeepInstance bool `db:"keep_instance"`
}

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

// createMachineArgs represents the struct to be used for the input parameters
// of the createMachine state method in the machine domain.
type createMachineArgs struct {
	name        machine.Name
	machineUUID machine.UUID
	netNodeUUID string
	parentName  machine.Name
	nonce       *string
}

// lxdProfile represents the struct to be used for the sqlair statements on the
// lxd_profile table.
type lxdProfile struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	Name        string       `db:"name"`
	Index       int          `db:"array_index"`
}

type machineNonce struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	Nonce       string       `db:"nonce"`
}

type machineInstance struct {
	MachineName string `db:"machine_name"`
	InstanceID  string `db:"instance_id"`
	IsContainer int64  `db:"is_container"`
}

type createMachine struct {
	Name        string           `db:"name"`
	NetNodeUUID string           `db:"net_node_uuid"`
	UUID        string           `db:"uuid"`
	Nonce       sql.Null[string] `db:"nonce"`
	LifeID      int64            `db:"life_id"`
}

type machinePlatformUUID struct {
	MachineUUID    machine.UUID     `db:"machine_uuid"`
	OSID           sql.Null[int64]  `db:"os_id"`
	Channel        sql.Null[string] `db:"channel"`
	ArchitectureID int              `db:"architecture_id"`
}

type netNodeUUID struct {
	NetNodeUUID string `db:"uuid"`
}

type machineNameWithNetNodeUUID struct {
	Name        machine.Name `db:"name"`
	NetNodeUUID string       `db:"net_node_uuid"`
}

type machineNameWithMachineUUID struct {
	Name machine.Name `db:"name"`
	UUID machine.UUID `db:"uuid"`
}

type machinePlacement struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	ScopeID     int          `db:"scope_id"`
	Directive   string       `db:"directive"`
}
