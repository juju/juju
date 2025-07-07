// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "github.com/juju/juju/core/network"

// NetAddr represents an IP address and its
// association with a network interface.
type NetAddr struct {
	InterfaceName string
	ProviderID    *network.Id
	// AddressValue is the IP address.
	// It *must* include a suffix indicating the subnet mask.
	AddressValue string
	// ProviderSubnetID is the provider's identity for the subnet that this
	// address is connected to. It is intended to uniquely identify the
	// subnet in the event that the same CIDR is used on multiple provider
	// networks.
	ProviderSubnetID *network.Id
	AddressType      network.AddressType
	ConfigType       network.AddressConfigType
	// Origin identifies the authority of this address.
	// I.e. the machine itself, or the provider substrate.
	Origin      network.Origin
	Scope       network.Scope
	IsSecondary bool
	IsShadow    bool
	Space       string
}

// NetInterface represents a physical or virtual
// network interface and its IP addresses.
type NetInterface struct {
	Name             string
	MTU              *int64
	MACAddress       *string
	ProviderID       *network.Id
	Type             network.LinkLayerDeviceType
	VirtualPortType  network.VirtualPortType
	IsAutoStart      bool
	IsEnabled        bool
	ParentDeviceName string
	GatewayAddress   *string
	IsDefaultGateway bool
	VLANTag          uint64
	DNSSearchDomains []string
	DNSAddresses     []string

	Addrs []NetAddr

	// Note (manadart 2025-04-29): Although we capture provider VLAN IDs and
	// routes, and send them over the wire, we never stored them in Mongo.
	// Accordingly, we eschew setting them in Dqlite, but they should be added
	// here and handled by network config updates if they become pertinent.
}

// DeviceToBridge indicates a device on a known machine that should be bridged
// in order to provision a container or virtual machine on it with appropriate
// network connectivity.
// It is the result of factoring space constraints and/or bindings of the
// application to be deployed into the container or virtual machine.
type DeviceToBridge struct {
	// DeviceName is the name of the device on the machine that should
	// be bridged.
	DeviceName string

	// BridgeName is the name of the bridge that we want created.
	BridgeName string

	// MACAddress is the MAC address of the device to be bridged
	MACAddress string
}

// DeviceType represents the type of a link layer device, as recorded
// in the link_layer_device_type lookup table.
type DeviceType int

const (
	DeviceTypeUnknown DeviceType = iota
	DeviceTypeLoopback
	DeviceTypeEthernet
	DeviceType8021q
	DeviceTypeBond
	DeviceTypeBridge
	DeviceTypeVXLAN
)

// VirtualPortType represents the type of a link layer device port, as
// recorded in the virtual_port_type lookup table.
type VirtualPortType int

const (
	NonVirtualPortType VirtualPortType = iota
	OpenVswitchVirtualPortType
)

// UnitNetwork represents network relationship details for a specific endpoint.
type UnitNetwork struct {

	// EndpointName specifies the name of the network endpoint associated with
	// the current UnitNetwork instance.
	EndpointName string

	// DeviceInfos is a collection of device-specific information
	// associated with an endpoint.
	DeviceInfos []DeviceInfo

	// IngressAddresses represents the list of ingress addresses
	// associated with a network endpoint.
	IngressAddresses []string

	// EgressSubnets specifies a list of subnets used for outgoing network
	// traffic from the endpoint.
	EgressSubnets []string
}

// DeviceInfo represents information about a network device.
type DeviceInfo struct {
	// Name specifies the network device's human-readable name or identifier.
	Name string

	// MACAddress specifies the hardware MAC address of a network
	// interface (e.g., "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// Addresses specifies a list of network addresses and related information
	// associated with the device.
	Addresses []AddressInfo
}

// AddressInfo represents network address details.
type AddressInfo struct {

	// Hostname defines the DNS-resolvable name of the network address,
	// typically used for identifying a device or service.
	Hostname string

	// Value represents a string value, often used to specify network-related
	// information or data.
	Value string

	// CIDR specifies the subnet in CIDR notation, indicating the network
	// portion of the associated IP address.
	CIDR string
}

// MovedSubnets represents a subnet that has been moved from one network space
// to another.
type MovedSubnets struct {
	// UUID is the unique identifier of the subnet.
	UUID SubnetUUID
	// CIDR specifies the subnet range in CIDR notation.
	CIDR string
	// FromSpace identifies the source network space from which the subnet was moved.
	FromSpace network.SpaceName
	// ToSpace identifies the destination network space to which the subnet was moved.
	ToSpace network.SpaceName
}
