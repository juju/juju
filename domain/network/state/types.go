// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/core/network"
)

// Subnet represents a single row from the subnet table.
type Subnet struct {
	// UUID is the subnet's UUID.
	UUID string `db:"uuid"`
	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string `db:"cidr"`
	// VLANtag is the subnet's vlan tag.
	VLANtag int `db:"vlan_tag"`
	// SpaceUUID is the space UUID.
	SpaceUUID string `db:"space_uuid"`
	// SubnetType indicates if the subnet is a fan overlay or a base subnet.
	SubnetType int `db:"subnet_type_id"`
}

// ProviderSubnet represents a single row from the provider_subnet table.
type ProviderSubnet struct {
	// SubnetUUID is the UUID of the subnet.
	SubnetUUID string `db:"subnet_uuid"`
	// ProviderNetworkId is the provider ID of the network
	// containing this subnet.
	ProviderNetworkId network.Id `db:"provider_id"`
}

// ProviderNetwork represents a single row from the provider_network table.
type ProviderNetwork struct {
	// ProviderNetworkUUID is the provider network UUID.
	ProviderNetworkUUID string `db:"uuid"`
	// ProviderNetworkId is the provider ID of the network.
	// containing this subnet.
	ProviderNetworkId network.Id `db:"provider_network_id"`
}

// ProviderNetworkSubnet represents a single row from the provider_network_subnet mapping table.
type ProviderNetworkSubnet struct {
	// SubnetUUID is the UUID of the subnet.
	SubnetUUID string `db:"subnet_uuid"`
	// ProviderNetworkUUID is the provider network UUID.
	ProviderNetworkUUID string `db:"provider_network_uuid"`
}

// Space represents a single row from the space table.
type Space struct {
	// Name is the space name.
	Name string `db:"name"`
	// UUID is the unique ID of the space.
	UUID string `db:"uuid"`
}

// ProviderSpace represents a single row from the provider_space table.
type ProviderSpace struct {
	// SpaceUUID is the unique ID of the space.
	SpaceUUID string `db:"space_uuid"`
	// ProviderId is a provider-specific subnet ID.
	ProviderId network.Id `db:"provider_id"`
}

// SpaceSubnetRow represents a single row from the database when
// space is joined with provider_space, subnet, subnet_type,
// subnet_association, subject_subnet_type_uuid, subnet_association_type,
// provider_subnet, provider_network, provider_network_subnet,
// availability_zone and availability_zone_subnet.
// This type is also used for deserializing only subnets, which has the same
// fields except UUID, Name and ProviderID.
type SpaceSubnetRow struct {
	// UUID is the space UUID.
	UUID string `db:"uuid"`

	// Name is the space name.
	Name string `db:"name"`

	// ProviderID is the space provider id.
	ProviderID sql.NullString `db:"provider_id"`

	// Subnet fields

	// SubnetUUID is the subnet SubnetUUID.
	SubnetUUID string `db:"subnet_uuid"`

	// SubnetCIDR is the one of the subnet's cidr.
	SubnetCIDR string `db:"subnet_cidr"`

	// SubnetVLANTag is the subnet's vlan tag.
	SubnetVLANTag int `db:"subnet_vlan_tag"`

	// SubnetProviderID is the subnet's provider id.
	SubnetProviderID string `db:"subnet_provider_id"`

	// SubnetProviderNetworkID is the subnet's provider network id.
	SubnetProviderNetworkID string `db:"subnet_provider_network_id"`

	// SubnetProviderSpaceUUID is the subnet's space uuid.
	SubnetProviderSpaceUUID sql.NullString `db:"subnet_provider_space_uuid"`

	// SubnetSpaceUUID is the space uuid.
	SubnetSpaceUUID sql.NullString `db:"subnet_space_uuid"`

	// SubnetSpaceName is the name of the space the subnet is associated with.
	SubnetSpaceName sql.NullString `db:"subnet_space_name"`

	// SubnetAZ is the availability zones on the subnet.
	SubnetAZ string `db:"subnet_az"`

	// SubnetOverlayCIDR is the subnet's overlay cidr in a fan setup.
	SubnetOverlayCIDR sql.NullString `db:"subnet_overlay_cidr"`

	// SubnetUnderlayCIDR is the subnet's underlay cidr in a fan setup.
	SubnetUnderlayCIDR sql.NullString `db:"subnet_underlay_cidr"`
}

// Alias type to a slice of Space/Subnet rows.
type SpaceSubnetRows []SpaceSubnetRow

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

	for _, space := range sp {
		spInfo := network.SpaceInfo{
			ID:   space.UUID,
			Name: network.SpaceName(space.Name),
		}

		if space.ProviderID.Valid {
			spInfo.ProviderId = network.Id(space.ProviderID.String)
		}
		uniqueSpaces[space.UUID] = spInfo

		snInfo := space.ToSubnetInfo()
		if snInfo != nil {
			if _, ok := uniqueSubnets[space.UUID]; !ok {
				uniqueSubnets[space.UUID] = make(map[string]network.SubnetInfo)
			}

			snInfo.SpaceID = space.UUID
			snInfo.SpaceName = space.Name

			if space.ProviderID.Valid {
				snInfo.ProviderSpaceId = network.Id(space.ProviderID.String)
			}
			uniqueSubnets[space.UUID][space.SubnetUUID] = *snInfo

			if _, ok := uniqueAZs[space.UUID]; !ok {
				uniqueAZs[space.UUID] = make(map[string]map[string]string)
			}
			if _, ok := uniqueAZs[space.UUID][space.SubnetUUID]; !ok {
				uniqueAZs[space.UUID][space.SubnetUUID] = make(map[string]string)
			}
			uniqueAZs[space.UUID][space.SubnetUUID][space.SubnetAZ] = space.SubnetAZ
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
func (s SpaceSubnetRow) ToSubnetInfo() *network.SubnetInfo {
	// Make sure we don't add empty rows as empty subnets.
	if s.SubnetUUID == "" {
		return nil
	}
	sInfo := network.SubnetInfo{
		ID:                network.Id(s.SubnetUUID),
		CIDR:              s.SubnetCIDR,
		VLANTag:           s.SubnetVLANTag,
		ProviderId:        network.Id(s.SubnetProviderID),
		ProviderNetworkId: network.Id(s.SubnetProviderNetworkID),
	}
	if s.SubnetProviderSpaceUUID.Valid {
		sInfo.ProviderSpaceId = network.Id(s.SubnetProviderSpaceUUID.String)
	}
	if s.SubnetUnderlayCIDR.Valid {
		underlay := ""
		if s.SubnetUnderlayCIDR.Valid {
			underlay = s.SubnetUnderlayCIDR.String
		}
		sInfo.SetFan(underlay, "")
	}
	if s.SubnetSpaceUUID.Valid {
		sInfo.SpaceID = s.SubnetSpaceUUID.String
	}
	if s.SubnetSpaceName.Valid {
		sInfo.SpaceName = s.SubnetSpaceName.String
	}

	return &sInfo
}

// ToSubnetInfos converts Subnets to a slice of network.SubnetInfo structs.
// This method makes sure only unique AZs are mapped and flattens them into
// each subnet.
// No sorting is applied.
func (sn SpaceSubnetRows) ToSubnetInfos() network.SubnetInfos {
	// Prepare structs for unique subnets.
	uniqueAZs := make(map[string]map[string]string)
	uniqueSubnets := make(map[string]network.SubnetInfo)

	for _, subnet := range sn {
		subnetInfo := subnet.ToSubnetInfo()
		if subnetInfo != nil {
			uniqueSubnets[subnet.SubnetUUID] = *subnetInfo

			if _, ok := uniqueAZs[subnet.SubnetUUID]; !ok {
				uniqueAZs[subnet.SubnetUUID] = make(map[string]string)
			}
			uniqueAZs[subnet.SubnetUUID][subnet.SubnetAZ] = subnet.SubnetAZ
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
