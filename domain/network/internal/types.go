// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corenetwork "github.com/juju/juju/core/network"
)

// ImportLinkLayerDevice represents a physical or virtual
// network interface and its IP addresses.
type ImportLinkLayerDevice struct {
	UUID             string
	IsAutoStart      bool
	IsEnabled        bool
	MTU              *int64
	MachineID        string
	MACAddress       *string
	NetNodeUUID      string
	Name             string
	ParentDeviceName string
	ProviderID       *string
	Type             corenetwork.LinkLayerDeviceType
	VirtualPortType  corenetwork.VirtualPortType
}

// SpaceName represents a space's name and its unique identifier.
type SpaceName struct {
	// UUID is the unique identifier for the space.
	UUID string
	// Name is the human-readable name of the space.
	Name string
}
