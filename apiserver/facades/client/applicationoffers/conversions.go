// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

// paramsFromProviderSpaceInfo converts a ProviderSpaceInfo into the
// equivalent params.RemoteSpace.
func paramsFromProviderSpaceInfo(info *environs.ProviderSpaceInfo) params.RemoteSpace {
	result := params.RemoteSpace{
		CloudType:          info.CloudType,
		Name:               string(info.Name),
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
