// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/network"
)

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/network/service State,ProviderWithNetworking,ProviderWithZones

// defaultRouteDeviceAddress returns a cloud-local IPv4 device address on
// the input device. The device carries the default route, via the gateway
// 10.1.0.1.
func defaultRouteDeviceAddress(
	value, cidr, deviceUUID string,
) network.DeviceAddress {
	return network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      value,
				CIDR:       cidr,
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		DeviceUUID:             deviceUUID,
		IsDefaultGatewayDevice: true,
		GatewayAddress:         "10.1.0.1",
	}
}
