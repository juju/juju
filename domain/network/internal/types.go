// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
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
	Type             network.DeviceType
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

// EndpointNetworkInfo represents unit addresses and selected ingress
// addresses associated with an endpoint.
type EndpointNetworkInfo struct {
	// EndpointName specifies the name of the network endpoint.
	EndpointName string

	// Addresses is the set of unit addresses available on the endpoint.
	Addresses []UnitAddress

	// IngressAddresses is the ordered set of ingress addresses for the
	// endpoint.
	IngressAddresses []string
}

// UnitNetworkInfo represents unit addresses and selected ingress addresses
// for a unit when endpoint bindings are not available.
type UnitNetworkInfo struct {
	// Addresses is the set of unit addresses available on the unit.
	Addresses []UnitAddress

	// IngressAddresses is the ordered set of ingress addresses for the unit.
	IngressAddresses []string
}

// UnitAddress represents a unit address together with device metadata.
type UnitAddress struct {
	corenetwork.SpaceAddress

	// DeviceName specifies the network device's human-readable identifier.
	DeviceName string

	// MACAddress specifies the device's hardware MAC address.
	MACAddress string

	// DeviceType specifies the link-layer type of the device.
	DeviceType corenetwork.LinkLayerDeviceType
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
