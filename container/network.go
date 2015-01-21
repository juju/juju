// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/network"
)

const (
	// BridgeNetwork will have the container use the network bridge.
	BridgeNetwork = "bridge"
	// PhyscialNetwork will have the container use a specified network device.
	PhysicalNetwork = "physical"
)

// NetworkConfig defines how the container network will be configured.
type NetworkConfig struct {
	NetworkType string
	Device      string

	Interfaces []network.InterfaceInfo
}

// BridgeNetworkConfig returns a valid NetworkConfig to use the
// specified device as a network bridge for the container. It also
// allows passing in specific configuration for the container's
// network interfaces. If interfaces is nil the default configuration
// is used for the respective container type.
func BridgeNetworkConfig(device string, interfaces []network.InterfaceInfo) *NetworkConfig {
	return &NetworkConfig{BridgeNetwork, device, interfaces}
}

// PhysicalNetworkConfig returns a valid NetworkConfig to use the
// specified device as the network device for the container. It also
// allows passing in specific configuration for the container's
// network interfaces. If interfaces is nil the default configuration
// is used for the respective container type.
func PhysicalNetworkConfig(device string, interfaces []network.InterfaceInfo) *NetworkConfig {
	return &NetworkConfig{PhysicalNetwork, device, interfaces}
}
