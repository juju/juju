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
	Addresses        []ImportIPAddress
}

// ImportIPAddress represents an IP address with its configuration
// details for import operations.
type ImportIPAddress struct {
	UUID string                  // generated during import
	Type corenetwork.AddressType // deduced from AddressValue during import

	Scope            corenetwork.Scope
	AddressValue     string
	SubnetCIDR       string
	ConfigType       corenetwork.AddressConfigType
	IsSecondary      bool
	IsShadow         bool
	Origin           corenetwork.Origin
	ProviderID       *string
	ProviderSubnetID *string

	SubnetUUID string // deduced from provider subnet id or from subnetCIDR
}

// SpaceName represents a space's name and its unique identifier.
type SpaceName struct {
	// UUID is the unique identifier for the space.
	UUID string
	// Name is the human-readable name of the space.
	Name string
}

type ImportCloudService struct {
	UUID        string // generated during import
	DeviceUUID  string // generated during import
	NetNodeUUID string // generated during import

	ApplicationName string
	ProviderID      string
	Addresses       []ImportCloudServiceAddress
}

type ImportCloudServiceAddress struct {
	UUID string // generated during import

	Value   string
	Type    string
	Scope   string
	Origin  string
	SpaceID string
}
