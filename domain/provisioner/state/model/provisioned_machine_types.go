// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
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

// provLLDRow is the write type for link_layer_device.
type provLLDRow struct {
	UUID              string  `db:"uuid"`
	NetNodeUUID       string  `db:"net_node_uuid"`
	Name              string  `db:"name"`
	MTU               *int64  `db:"mtu"`
	MACAddress        *string `db:"mac_address"`
	DeviceTypeID      int     `db:"device_type_id"`
	VirtualPortTypeID int     `db:"virtual_port_type_id"`
	IsAutoStart       bool    `db:"is_auto_start"`
	IsEnabled         bool    `db:"is_enabled"`
	IsDefaultGateway  bool    `db:"is_default_gateway"`
	GatewayAddress    *string `db:"gateway_address"`
	VlanTag           uint64  `db:"vlan_tag"`
}

// provIPAddrRow is the write type for ip_address.
type provIPAddrRow struct {
	UUID         string  `db:"uuid"`
	NodeUUID     string  `db:"net_node_uuid"`
	DeviceUUID   string  `db:"device_uuid"`
	AddressValue string  `db:"address_value"`
	SubnetUUID   *string `db:"subnet_uuid"`
	TypeID       int     `db:"type_id"`
	ConfigTypeID int     `db:"config_type_id"`
	OriginID     int     `db:"origin_id"`
	ScopeID      int     `db:"scope_id"`
	IsSecondary  bool    `db:"is_secondary"`
	IsShadow     bool    `db:"is_shadow"`
}

// provProviderLLDRow is the write type for provider_link_layer_device.
type provProviderLLDRow struct {
	ProviderID string `db:"provider_id"`
	DeviceUUID string `db:"device_uuid"`
}

// provProviderIPRow is the write type for provider_ip_address.
type provProviderIPRow struct {
	ProviderID  string `db:"provider_id"`
	AddressUUID string `db:"address_uuid"`
}

// provDNSDomainRow is the write type for link_layer_device_dns_domain.
type provDNSDomainRow struct {
	DeviceUUID   string `db:"device_uuid"`
	SearchDomain string `db:"search_domain"`
}

// provDNSAddrRow is the write type for link_layer_device_dns_address.
type provDNSAddrRow struct {
	DeviceUUID string `db:"device_uuid"`
	Address    string `db:"dns_address"`
}

// provLLDParentRow is the write type for link_layer_device_parent.
type provLLDParentRow struct {
	DeviceUUID string `db:"device_uuid"`
	ParentUUID string `db:"parent_uuid"`
}

// provSubnetUUID holds a resolved subnet UUID.
type provSubnetUUID struct {
	UUID string `db:"uuid"`
}

// provProviderID is a query parameter for provider IDs.
type provProviderID struct {
	ProviderID string `db:"provider_id"`
}

// provLookupRow represents a single row from a network lookup table.
type provLookupRow struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// provNetConfigLookups holds enum ID maps for network config lookup tables.
type provNetConfigLookups struct {
	deviceType      map[corenetwork.LinkLayerDeviceType]int
	virtualPortType map[corenetwork.VirtualPortType]int
	addrType        map[corenetwork.AddressType]int
	addrConfigType  map[corenetwork.AddressConfigType]int
	origin          map[corenetwork.Origin]int
	scope           map[corenetwork.Scope]int
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

// provLLDNameUUID is used to read back name→UUID for existing devices.
type provLLDNameUUID struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// provIPAddrNameUUID is used to read back address_value→UUID for existing
// addresses.
type provIPAddrNameUUID struct {
	UUID  string `db:"uuid"`
	Value string `db:"address_value"`
}
