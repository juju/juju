// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/core/network"
)

// Space represents a single row from the database when
// space is joined with provider_space, subnet, subnet_type,
// subnet_association, subject_subnet_type_uuid, subnet_association_type,
// provider_subnet, provider_network, provider_network_subnet,
// availability_zone and availability_zone_subnet.
type Space struct {
	// UUID is the space UUID.
	UUID string `db:"uuid"`

	// Name is the space name.
	Name string `db:"name"`

	// ProviderID is the space provider id.
	ProviderID sql.NullString `db:"provider_id"`

	// SubnetUUID is one of the space's subnet id.
	SubnetUUID string `db:"subnet_uuid"`

	// CIDR is one of the space's subnet cidr.
	CIDR string `db:"subnet_cidr"`

	// VLANTag is one of the space's subnet vlan tag.
	VLANTag int `db:"vlan_tag"`

	// SubnetProviderID is one of the space's subnet provider id.
	SubnetProviderID string `db:"subnet_provider_id"`

	// SubnetProviderNetworkID is one of the space's subnet provider network id.
	SubnetProviderNetworkID string `db:"subnet_provider_network_id"`

	// SubnetAZ is one of the availability zones on one of the subnets of the space.
	SubnetAZ string `db:"subnet_az"`

	// SubnetOverlayCIDR is one of the space's subnet overlay cidr in a fan setup.
	SubnetOverlayCIDR sql.NullString `db:"subnet_overlay_cidr"`

	// SubnetUnderlayCIDR is one of the space's subnet underlay cidr in a fan setup.
	SubnetUnderlayCIDR sql.NullString `db:"subnet_underlay_cidr"`
}

// Alias type to a slice of Space('s).
type Spaces []Space

// ToSpaceInfos converts Spaces to a slice of network.SpaceInfo structs.
// This method makes sure only unique subnets are mapped and flattens them into
// each space.
// No sorting is applied.
func (sp Spaces) ToSpaceInfos() network.SpaceInfos {
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

		if _, ok := uniqueSubnets[space.UUID]; !ok {
			uniqueSubnets[space.UUID] = make(map[string]network.SubnetInfo)
		}
		snInfo := network.SubnetInfo{
			ID:                network.Id(space.SubnetUUID),
			CIDR:              space.CIDR,
			ProviderId:        network.Id(space.SubnetProviderID),
			ProviderNetworkId: network.Id(space.SubnetProviderNetworkID),
			VLANTag:           space.VLANTag,
			SpaceID:           space.UUID,
			SpaceName:         space.Name,
		}
		if space.ProviderID.Valid {
			spInfo.ProviderId = network.Id(space.ProviderID.String)
			snInfo.ProviderSpaceId = network.Id(space.ProviderID.String)
		}
		if space.SubnetOverlayCIDR.Valid || space.SubnetUnderlayCIDR.Valid {
			underlay := ""
			if space.SubnetUnderlayCIDR.Valid {
				underlay = space.SubnetUnderlayCIDR.String
			}
			overlay := ""
			if space.SubnetOverlayCIDR.Valid {
				overlay = space.SubnetOverlayCIDR.String
			}
			snInfo.SetFan(underlay, overlay)
		}

		uniqueSpaces[space.UUID] = spInfo
		uniqueSubnets[space.UUID][space.SubnetUUID] = snInfo

		if _, ok := uniqueAZs[space.UUID]; !ok {
			uniqueAZs[space.UUID] = make(map[string]map[string]string)
		}
		if _, ok := uniqueAZs[space.UUID][space.SubnetUUID]; !ok {
			uniqueAZs[space.UUID][space.SubnetUUID] = make(map[string]string)
		}
		uniqueAZs[space.UUID][space.SubnetUUID][space.SubnetAZ] = space.SubnetAZ
	}

	// Iterate through every space and flatten its subnets.
	for spaceUUID, space := range uniqueSpaces {
		var subnets network.SubnetInfos
		// Iterate through every subnet and flatten its availability zones.
		for subnetUUID, subnet := range uniqueSubnets[spaceUUID] {
			var availabilityZones []string
			for _, availabilityZone := range uniqueAZs[spaceUUID][subnetUUID] {
				availabilityZones = append(availabilityZones, availabilityZone)
			}
			subnet.AvailabilityZones = availabilityZones

			subnets = append(subnets, subnet)
		}
		space.Subnets = subnets

		res = append(res, space)
	}

	return res
}

// Subnet represents a single row from the database when
// subnet is joined with subnet_type, subnet_association,
// subject_subnet_type_uuid, subnet_association_type, provider_subnet,
// provider_network, provider_network_subnet, availability_zone and
// availability_zone_subnet.
type Subnet struct {
	// UUID is the subnet UUID.
	UUID string `db:"uuid"`

	// CIDR is the one of the subnet's cidr.
	CIDR string `db:"cidr"`

	// VLANTag is the subnet's vlan tag.
	VLANTag int `db:"vlan_tag"`

	// ProviderID is the subnet's provider id.
	ProviderID string `db:"provider_id"`

	// ProviderNetworkID is the subnet's provider network id.
	ProviderNetworkID string `db:"provider_network_id"`

	// ProviderSpaceUUID is the subnet's space uuid.
	ProviderSpaceUUID sql.NullString `db:"provider_space_uuid"`

	// SpaceUUID is the space uuid.
	SpaceUUID sql.NullString `db:"space_uuid"`

	// SpaceName is the name of the space the subnet is associated with.
	SpaceName sql.NullString `db:"space_name"`

	// AZ is the availability zones on the subnet.
	AZ string `db:"az"`

	// OverlayCIDR is the subnet's overlay cidr in a fan setup.
	OverlayCIDR sql.NullString `db:"overlay_cidr"`

	// UnderlayCIDR is the subnet's underlay cidr in a fan setup.
	UnderlayCIDR sql.NullString `db:"underlay_cidr"`
}

// Alias type to a slice of Subnet('s).
type Subnets []Subnet

// ToSubnetInfos converts Subnets to a slice of network.SubnetInfo structs.
// This method makes sure only unique AZs are mapped and flattens them into
// each subnet.
// No sorting is applied.
func (sn Subnets) ToSubnetInfos() network.SubnetInfos {
	var res network.SubnetInfos

	// Prepare structs for unique subnets.
	uniqueAZs := make(map[string]map[string]string)
	uniqueSubnets := make(map[string]network.SubnetInfo)

	for _, subnet := range sn {
		sInfo := network.SubnetInfo{
			ID:                network.Id(subnet.UUID),
			CIDR:              subnet.CIDR,
			VLANTag:           subnet.VLANTag,
			ProviderId:        network.Id(subnet.ProviderID),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkID),
		}
		if subnet.ProviderSpaceUUID.Valid {
			sInfo.ProviderSpaceId = network.Id(subnet.ProviderSpaceUUID.String)
		}
		if subnet.OverlayCIDR.Valid || subnet.UnderlayCIDR.Valid {
			underlay := ""
			if subnet.UnderlayCIDR.Valid {
				underlay = subnet.UnderlayCIDR.String
			}
			overlay := ""
			if subnet.OverlayCIDR.Valid {
				overlay = subnet.OverlayCIDR.String
			}
			sInfo.SetFan(underlay, overlay)
		}
		if subnet.SpaceUUID.Valid {
			sInfo.SpaceID = subnet.SpaceUUID.String
		}
		if subnet.SpaceName.Valid {
			sInfo.SpaceName = subnet.SpaceName.String
		}
		uniqueSubnets[subnet.UUID] = sInfo

		if _, ok := uniqueAZs[subnet.UUID]; !ok {
			uniqueAZs[subnet.UUID] = make(map[string]string)
		}
		uniqueAZs[subnet.UUID][subnet.AZ] = subnet.AZ
	}

	// Iterate through every subnet and flatten its AZs.
	for subnetUUID, subnet := range uniqueSubnets {
		var availabilityZones []string
		for _, availabilityZone := range uniqueAZs[subnetUUID] {
			availabilityZones = append(availabilityZones, availabilityZone)
		}
		subnet.AvailabilityZones = availabilityZones

		res = append(res, subnet)
	}

	return res
}
