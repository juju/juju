// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

// paramsFromProviderSpaceInfo converts a ProviderSpaceInfo into the
// equivalent params.RemoteSpace.
func paramsFromProviderSpaceInfo(info *environs.ProviderSpaceInfo) params.RemoteSpace {
	result := params.RemoteSpace{
		CloudType:          info.CloudType,
		Name:               info.Name,
		ProviderId:         string(info.ProviderId),
		ProviderAttributes: info.ProviderAttributes,
	}
	for _, subnet := range info.Subnets {
		resultSubnet := params.Subnet{
			CIDR:              subnet.CIDR,
			ProviderId:        string(subnet.ProviderId),
			ProviderNetworkId: string(subnet.ProviderNetworkId),
			ProviderSpaceId:   string(subnet.ProviderSpaceId),
			VLANTag:           subnet.VLANTag,
			Zones:             subnet.AvailabilityZones,
		}
		result.Subnets = append(result.Subnets, resultSubnet)
	}
	return result
}

// providerSpaceInfoFromParams converts a params.RemoteSpace to the
// equivalent ProviderSpaceInfo.
func providerSpaceInfoFromParams(space params.RemoteSpace) *environs.ProviderSpaceInfo {
	result := &environs.ProviderSpaceInfo{
		CloudType:          space.CloudType,
		ProviderAttributes: space.ProviderAttributes,
		SpaceInfo: network.SpaceInfo{
			Name:       space.Name,
			ProviderId: network.Id(space.ProviderId),
		},
	}
	for _, subnet := range space.Subnets {
		resultSubnet := network.SubnetInfo{
			CIDR:              subnet.CIDR,
			ProviderId:        network.Id(subnet.ProviderId),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkId),
			ProviderSpaceId:   network.Id(subnet.ProviderSpaceId),
			VLANTag:           subnet.VLANTag,
			AvailabilityZones: subnet.Zones,
		}
		result.Subnets = append(result.Subnets, resultSubnet)
	}
	return result
}

// spaceInfoFromState converts a state.Space into the equivalent
// network.SpaceInfo.
func spaceInfoFromState(space Space) (*network.SpaceInfo, error) {
	result := &network.SpaceInfo{
		Name:       space.Name(),
		ProviderId: space.ProviderId(),
	}
	subnets, err := space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, subnet := range subnets {
		resultSubnet := network.SubnetInfo{
			CIDR:              subnet.CIDR(),
			ProviderId:        subnet.ProviderId(),
			ProviderNetworkId: subnet.ProviderNetworkId(),
			VLANTag:           subnet.VLANTag(),
			AvailabilityZones: subnet.AvailabilityZones(),
		}
		result.Subnets = append(result.Subnets, resultSubnet)
	}
	return result, nil
}
