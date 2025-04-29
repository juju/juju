// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/core/network"
)

// subnet represents a single row from the subnet table.
type subnet struct {
	// UUID is the subnet's UUID.
	UUID string `db:"uuid"`
	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string `db:"cidr"`
	// VLANtag is the subnet's vlan tag.
	VLANtag int `db:"vlan_tag"`
	// SpaceUUID is the space UUID.
	SpaceUUID string `db:"space_uuid"`
}

// providerSubnet represents a single row from the provider_subnet table.
type providerSubnet struct {
	// SubnetUUID is the UUID of the subnet.
	SubnetUUID string `db:"subnet_uuid"`
	// ProviderID is the provider-specific subnet ID.
	ProviderID network.Id `db:"provider_id"`
}

// providerNetwork represents a single row from the provider_network table.
type providerNetwork struct {
	// ProviderNetworkUUID is the provider network UUID.
	ProviderNetworkUUID string `db:"uuid"`
	// ProviderNetworkID is the provider-specific ID of the network
	// containing this subnet.
	ProviderNetworkID network.Id `db:"provider_network_id"`
}

// providerNetworkSubnet represents a single row from the provider_network_subnet mapping table.
type providerNetworkSubnet struct {
	// SubnetUUID is the UUID of the subnet.
	SubnetUUID string `db:"subnet_uuid"`
	// ProviderNetworkUUID is the provider network UUID.
	ProviderNetworkUUID string `db:"provider_network_uuid"`
}

// space represents a single row from the space table.
type space struct {
	// Name is the space name.
	Name string `db:"name"`
	// UUID is the unique ID of the space.
	UUID string `db:"uuid"`
}

type spaceName struct {
	Name string `db:"name"`
}

type countResult struct {
	Count int `db:"count"`
}

// providerSpace represents a single row from the provider_space table.
type providerSpace struct {
	// SpaceUUID is the unique ID of the space.
	SpaceUUID string `db:"space_uuid"`
	// ProviderID is a provider-specific space ID.
	ProviderID network.Id `db:"provider_id"`
}

// availabilityZone represents a row from the availability_zone table.
type availabilityZone struct {
	// Name is the name of the availability zone.
	Name string `db:"name"`
	// UUID is the unique ID of the availability zone.
	UUID string `db:"uuid"`
}

// availabilityZoneSubnet represents a row from the availability_zone_subnet
// table.
type availabilityZoneSubnet struct {
	// UUID is the unique ID of the availability zone.
	AZUUID string `db:"availability_zone_uuid"`
	// SubnetUUID is the unique ID of the Subnet.
	SubnetUUID string `db:"subnet_uuid"`
}

// SubnetRow represents the subnet fields of a single row from the
// v_space_subnets view.
type SubnetRow struct {
	// UUID is the subnet UUID.
	UUID string `db:"subnet_uuid"`

	// CIDR is the one of the subnet's cidr.
	CIDR string `db:"subnet_cidr"`

	// VLANTag is the subnet's vlan tag.
	VLANTag int `db:"subnet_vlan_tag"`

	// ProviderID is the subnet's provider id.
	ProviderID string `db:"subnet_provider_id"`

	// ProviderNetworkID is the subnet's provider network id.
	ProviderNetworkID string `db:"subnet_provider_network_id"`

	// ProviderSpaceUUID is the subnet's space uuid.
	ProviderSpaceUUID sql.NullString `db:"subnet_provider_space_uuid"`

	// SpaceUUID is the space uuid.
	SpaceUUID sql.NullString `db:"subnet_space_uuid"`

	// SpaceName is the name of the space the subnet is associated with.
	SpaceName sql.NullString `db:"subnet_space_name"`

	// AZ is the availability zones on the subnet.
	AZ string `db:"subnet_az"`
}

// SpaceSubnetRow represents a single row from the v_space_subnets view.
type spaceSubnetRow struct {
	// SubnetRow is embedded by SpaceSubnetRow since every row corresponds to a
	// subnet of the space. This allows SubnetRow to be
	SubnetRow

	// UUID is the space UUID.
	SpaceUUID string `db:"uuid"`

	// Name is the space name.
	SpaceName string `db:"name"`

	// ProviderID is the space provider id.
	SpaceProviderID sql.NullString `db:"provider_id"`
}

// SpaceSubnetRows is a slice of SpaceSubnet rows.
type SpaceSubnetRows []spaceSubnetRow

// subnetRows is a slice of Subnet rows.
type subnetRows []SubnetRow

// ToSpaceInfos converts Spaces to a slice of network.SpaceInfo structs.
// This method makes sure only unique subnets are mapped and flattens them into
// each space.
// No sorting is applied.
func (sp SpaceSubnetRows) ToSpaceInfos() network.SpaceInfos {
	var res network.SpaceInfos

	// Prepare structs for unique subnets for each space.
	uniqueAZs := make(map[string]map[string]map[string]string)
	uniqueSubnets := make(map[string]map[string]network.SubnetInfo)
	uniqueSpaces := make(map[string]network.SpaceInfo)

	for _, spaceSubnet := range sp {
		spInfo := network.SpaceInfo{
			ID:   spaceSubnet.SpaceUUID,
			Name: network.SpaceName(spaceSubnet.SpaceName),
		}

		if spaceSubnet.SpaceProviderID.Valid {
			spInfo.ProviderId = network.Id(spaceSubnet.SpaceProviderID.String)
		}
		uniqueSpaces[spaceSubnet.SpaceUUID] = spInfo

		snInfo := spaceSubnet.SubnetRow.ToSubnetInfo()
		if snInfo != nil {
			if _, ok := uniqueSubnets[spaceSubnet.SpaceUUID]; !ok {
				uniqueSubnets[spaceSubnet.SpaceUUID] = make(map[string]network.SubnetInfo)
			}

			uniqueSubnets[spaceSubnet.SpaceUUID][spaceSubnet.UUID] = *snInfo

			if _, ok := uniqueAZs[spaceSubnet.SpaceUUID]; !ok {
				uniqueAZs[spaceSubnet.SpaceUUID] = make(map[string]map[string]string)
			}
			if _, ok := uniqueAZs[spaceSubnet.SpaceUUID][spaceSubnet.UUID]; !ok {
				uniqueAZs[spaceSubnet.SpaceUUID][spaceSubnet.UUID] = make(map[string]string)
			}
			uniqueAZs[spaceSubnet.SpaceUUID][spaceSubnet.UUID][spaceSubnet.AZ] = spaceSubnet.AZ
		}
	}

	// Iterate through every space and flatten its subnets.
	for spaceUUID, space := range uniqueSpaces {
		space.Subnets = flattenAZs(uniqueSubnets[spaceUUID], uniqueAZs[spaceUUID])
		res = append(res, space)
	}

	return res
}

// ToSubnetInfo deserializes a row containing subnet fields to a SubnetInfo
// struct.
func (s SubnetRow) ToSubnetInfo() *network.SubnetInfo {
	// Make sure we don't add empty rows as empty subnets.
	if s.UUID == "" {
		return nil
	}
	sInfo := network.SubnetInfo{
		ID:                network.Id(s.UUID),
		CIDR:              s.CIDR,
		VLANTag:           s.VLANTag,
		ProviderId:        network.Id(s.ProviderID),
		ProviderNetworkId: network.Id(s.ProviderNetworkID),
	}
	if s.ProviderSpaceUUID.Valid {
		sInfo.ProviderSpaceId = network.Id(s.ProviderSpaceUUID.String)
	}
	if s.SpaceUUID.Valid {
		sInfo.SpaceID = s.SpaceUUID.String
	}
	if s.SpaceName.Valid {
		sInfo.SpaceName = s.SpaceName.String
	}

	return &sInfo
}

// ToSubnetInfos converts Subnets to a slice of network.SubnetInfo structs.
// This method makes sure only unique AZs are mapped and flattens them into
// each subnet.
// No sorting is applied.
func (sn subnetRows) ToSubnetInfos() network.SubnetInfos {
	// Prepare structs for unique subnets.
	uniqueAZs := make(map[string]map[string]string)
	uniqueSubnets := make(map[string]network.SubnetInfo)

	for _, subnet := range sn {
		subnetInfo := subnet.ToSubnetInfo()
		if subnetInfo != nil {
			uniqueSubnets[subnet.UUID] = *subnetInfo

			if _, ok := uniqueAZs[subnet.UUID]; !ok {
				uniqueAZs[subnet.UUID] = make(map[string]string)
			}
			uniqueAZs[subnet.UUID][subnet.AZ] = subnet.AZ
		}
	}

	return flattenAZs(uniqueSubnets, uniqueAZs)
}

// flattenAZs iterates through every subnet and flatten its AZs.
func flattenAZs(
	uniqueSubnets map[string]network.SubnetInfo,
	uniqueAZs map[string]map[string]string,
) network.SubnetInfos {
	var subnets network.SubnetInfos

	for subnetUUID, subnet := range uniqueSubnets {
		var availabilityZones []string
		for _, availabilityZone := range uniqueAZs[subnetUUID] {
			availabilityZones = append(availabilityZones, availabilityZone)
		}
		subnet.AvailabilityZones = availabilityZones

		subnets = append(subnets, subnet)
	}

	return subnets
}

// machineInterfaceRow is the type for a row from the v_machine_interface view.
type machineInterfaceRow struct {
	// MachineUUID and associated machine fields.
	MachineUUID string `db:"machine_uuid"`
	MachineName string `db:"machine_name"`
	NetNodeUUID string `db:"net_node_uuid"`

	// DeviceUUID and associated link-layer device fields.
	DeviceUUID        string         `db:"device_uuid"`
	DeviceName        string         `db:"device_name"`
	MTU               sql.NullInt64  `db:"mtu"`
	MacAddress        sql.NullString `db:"mac_address"`
	ProviderID        sql.NullString `db:"device_provider_id"`
	DeviceTypeID      int            `db:"device_type_id"`
	VirtualPortTypeID int            `db:"virtual_port_type_id"`
	IsAutoStart       bool           `db:"is_auto_start"`
	IsEnabled         bool           `db:"is_enabled"`
	ParentDeviceUUID  sql.NullString `db:"parent_device_uuid"`
	ParentDeviceName  sql.NullString `db:"parent_device_name"`
	GatewayAddress    sql.NullString `db:"gateway_address"`
	IsDefaultGateway  bool           `db:"is_default_gateway"`

	// AddressUUID and associated IP address fields.
	AddressUUID      sql.NullString `db:"address_uuid"`
	AddressValue     sql.NullString `db:"address_value"`
	SubnetUUID       sql.NullString `db:"subnet_uuid"`
	CIDR             sql.NullString `db:"cidr"`
	ProviderSubnetID sql.NullString `db:"provider_subnet_id"`
	AddressTypeID    sql.NullInt64  `db:"address_type_id"`
	ConfigTypeID     sql.NullInt64  `db:"config_type_id"`
	OriginID         sql.NullInt64  `db:"origin_id"`
	ScopeID          sql.NullInt64  `db:"scope_id"`
	IsSecondary      sql.NullBool   `db:"is_secondary"`
	IsShadow         sql.NullBool   `db:"is_shadow"`
}
