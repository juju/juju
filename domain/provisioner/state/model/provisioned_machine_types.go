// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"

	"github.com/juju/juju/core/instance"
)

// provMachineUUIDParam is a query parameter for a machine UUID.
type provMachineUUIDParam struct {
	UUID string `db:"uuid"`
}

// provNetNodeUUID holds the net_node_uuid returned when joining on machine.
type provNetNodeUUID struct {
	NetNodeUUID string `db:"net_node_uuid"`
}

// provInstanceID holds the instance_id column from machine_cloud_instance.
type provInstanceID struct {
	InstanceID string `db:"instance_id"`
}

// provInstanceData is the write type for machine_cloud_instance.
type provInstanceData struct {
	MachineUUID          string           `db:"machine_uuid"`
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

// provMachineNonce is the write type for the nonce UPDATE on machine.
type provMachineNonce struct {
	MachineUUID string `db:"machine_uuid"`
	Nonce       string `db:"nonce"`
}

// provInstanceTag is the write type for instance_tag.
type provInstanceTag struct {
	MachineUUID string `db:"machine_uuid"`
	Tag         string `db:"tag"`
}

// provAZName is used for AZ lookup and to carry the resolved UUID.
type provAZName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// provInstanceTagsFrom converts hardware characteristics to a slice of
// provInstanceTag rows, returning nil if there are no tags.
func provInstanceTagsFrom(machineUUID string, hc *instance.HardwareCharacteristics) []provInstanceTag {
	if hc == nil || hc.Tags == nil {
		return nil
	}
	res := make([]provInstanceTag, len(*hc.Tags))
	for i, t := range *hc.Tags {
		res[i] = provInstanceTag{MachineUUID: machineUUID, Tag: t}
	}
	return res
}

// provVolumeID is a query parameter for volume_id lookups.
type provVolumeID struct {
	VolumeID string `db:"volume_id"`
}

// provVolumeUUID holds a resolved volume UUID.
type provVolumeUUID struct {
	UUID string `db:"uuid"`
}

// provVolumeProvisionedInfo is the write type for updating storage_volume.
type provVolumeProvisionedInfo struct {
	UUID       string `db:"uuid"`
	ProviderID string `db:"provider_id"`
	SizeMiB    uint64 `db:"size_mib"`
	HardwareID string `db:"hardware_id"`
	WWN        string `db:"wwn"`
	Persistent bool   `db:"persistent"`
}

// provEntityUUID is a generic UUID holder for storage entities.
type provEntityUUID struct {
	UUID string `db:"uuid"`
}

// provBlockDeviceRow is the read type for block_device.
type provBlockDeviceRow struct {
	UUID        string `db:"uuid"`
	MachineUUID string `db:"machine_uuid"`
	Name        string `db:"name"`
	BusAddress  string `db:"bus_address"`
}

// provBlockDeviceLinkRow is the read/write type for block_device_link_device.
type provBlockDeviceLinkRow struct {
	BlockDeviceUUID string `db:"block_device_uuid"`
	MachineUUID     string `db:"machine_uuid"`
	LinkName        string `db:"name"`
}

// provNewBlockDeviceRow is the insert type for block_device.
type provNewBlockDeviceRow struct {
	UUID        string `db:"uuid"`
	MachineUUID string `db:"machine_uuid"`
	Name        string `db:"name"`
	BusAddress  string `db:"bus_address"`
}

// provAttachmentProvisionedInfo is the write type for storage_volume_attachment.
type provAttachmentProvisionedInfo struct {
	UUID            string           `db:"uuid"`
	ReadOnly        bool             `db:"read_only"`
	BlockDeviceUUID sql.Null[string] `db:"block_device_uuid"`
}

// provPlanProvisionedInfo is the write type for storage_volume_attachment_plan.
type provPlanProvisionedInfo struct {
	UUID         string `db:"uuid"`
	DeviceTypeID int    `db:"device_type_id"`
}

// provPlanUUIDParam is a query parameter for plan UUID DELETE's.
type provPlanUUIDParam struct {
	UUID string `db:"uuid"`
}

// provPlanAttrRow is the write type for storage_volume_attachment_plan_attr.
type provPlanAttrRow struct {
	PlanUUID string `db:"attachment_plan_uuid"`
	Key      string `db:"key"`
	Value    string `db:"value"`
}
