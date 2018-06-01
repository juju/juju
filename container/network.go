// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/network"
)

const (
	// BridgeNetwork will have the container use the network bridge.
	BridgeNetwork = "bridge"
)

// NetworkConfig defines how the container network will be configured.
type NetworkConfig struct {
	NetworkType string
	Device      string
	MTU         int

	Interfaces []network.InterfaceInfo
}

// FallbackInterfaceInfo returns a single "eth0" interface configured with DHCP.
func FallbackInterfaceInfo() []network.InterfaceInfo {
	return []network.InterfaceInfo{{
		InterfaceName: "eth0",
		InterfaceType: network.EthernetInterface,
		ConfigType:    network.ConfigDHCP,
	}}
}

// BridgeNetworkConfig returns a valid NetworkConfig to use the specified device
// as a network bridge for the container. It also allows passing in specific
// configuration for the container's network interfaces and default MTU to use.
// If interfaces is empty, FallbackInterfaceInfo() is used to get the a sane
// default
func BridgeNetworkConfig(device string, mtu int, interfaces []network.InterfaceInfo) *NetworkConfig {
	if len(interfaces) == 0 {
		interfaces = FallbackInterfaceInfo()
	}
	return &NetworkConfig{BridgeNetwork, device, mtu, interfaces}
}
