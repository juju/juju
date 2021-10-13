// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/core/network"
)

const (
	// BridgeNetwork will have the container use the network bridge.
	BridgeNetwork = "bridge"
)

// NetworkConfig defines how the container network will be configured.
type NetworkConfig struct {
	NetworkType string
	MTU         int

	Interfaces network.InterfaceInfos
}

// BridgeNetworkConfig returns a valid NetworkConfig to use the specified device
// as a network bridge for the container. It also allows passing in specific
// configuration for the container's network interfaces and default MTU to use.
func BridgeNetworkConfig(mtu int, interfaces network.InterfaceInfos) *NetworkConfig {
	return &NetworkConfig{BridgeNetwork, mtu, interfaces}
}
