// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
)

// ImportLinkLayerDevice represents a physical or virtual
// network interface and its IP addresses.
type ImportLinkLayerDevice struct {
	IsAutoStart      bool
	IsEnabled        bool
	MTU              *int64
	MachineID        machine.Name
	MACAddress       *string
	NetNodeUUID      corenetwork.NetNodeUUID
	Name             string
	ParentDeviceName string
	ProviderID       *corenetwork.Id
	Type             network.DeviceType
	VirtualPortType  network.VirtualPortType
}
