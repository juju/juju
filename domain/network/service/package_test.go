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

// machineLocalDefaultRouteAddresses returns the addresses of two devices:
//   - eth0 does not carry the default route. It has the cloud-local address
//     10.0.0.10.
//   - eth1 carries the default route, via the gateway 192.168.1.1. It has the
//     machine-local address 192.168.1.5, which is in the gateway's subnet, and
//     the cloud-local address 10.0.0.20, which is not.
func machineLocalDefaultRouteAddresses() (eth0, eth1MachineLocal, eth1CloudLocal network.DeviceAddress) {
	eth0 = defaultRouteDeviceAddress("10.0.0.10", "10.0.0.0/24", "eth0-uuid")
	eth0.IsDefaultGatewayDevice = false
	eth0.GatewayAddress = ""

	eth1MachineLocal = defaultRouteDeviceAddress("192.168.1.5", "192.168.1.0/24", "eth1-uuid")
	eth1MachineLocal.Scope = network.ScopeMachineLocal
	eth1MachineLocal.GatewayAddress = "192.168.1.1"

	eth1CloudLocal = defaultRouteDeviceAddress("10.0.0.20", "10.0.0.0/24", "eth1-uuid")
	eth1CloudLocal.GatewayAddress = "192.168.1.1"
	return eth0, eth1MachineLocal, eth1CloudLocal
}
