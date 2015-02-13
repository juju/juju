// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/network"
)

// params types.

// Network describes a single network available on an instance.
type Network struct {
	// Tag is the network's tag.
	Tag string `json:"Tag"`

	// ProviderId is the provider-specific network id.
	ProviderId string `json:"ProviderId"`

	// CIDR of the network, in "123.45.67.89/12" format.
	CIDR string `json:"CIDR"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"VLANTag"`
}

// NetworkInterface describes a single network interface available on
// an instance.
type NetworkInterface struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string `json:"MACAddress"`

	// InterfaceName is the OS-specific network device name (e.g.
	// "eth1", even for for a VLAN eth1.42 virtual interface).
	InterfaceName string `json:"InterfaceName"`

	// NetworkTag is this interface's network tag.
	NetworkTag string `json:"NetworkTag"`

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device.
	IsVirtual bool `json:"IsVirtual"`

	// Disabled returns whether the interface is disabled.
	Disabled bool `json:"Disabled"`
}

// NetworkInfo describes all the necessary information to configure
// all network interfaces on a machine. This mostly duplicates
// network.InterfaceInfo type and it's defined here so it can be kept
// separate and stable as definition to ensure proper wire-format for
// the API.
type NetworkInfo struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// NetworkName is juju-internal name of the network.
	// TODO(dimitern) This should be removed or adapted to the model
	// once spaces are introduced.
	NetworkName string

	// ProviderId is a provider-specific network id.
	ProviderId string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it at all or stop it if running.
	Disabled bool

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot. By default and for
	// backwards-compatibility, interfaces are configured to
	// auto-start.
	NoAutoStart bool `json:",omitempty"`

	// ConfigType, if set, defines what type of configuration to use.
	// See network.InterfaceConfigType for more info. If not set, for
	// backwards-compatibility, "dhcp" is assumed.
	ConfigType string `json:",omitempty"`

	// Address contains an optional static IP address to configure for
	// this network interface. The subnet mask to set will be inferred
	// from the CIDR value.
	Address string `json:",omitempty"`

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers []string `json:",omitempty"`

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually (one of) the host address(es).
	GatewayAddress string `json:",omitempty"`

	// ExtraConfig can contain any valid setting and its value allowed
	// inside an "iface" section of a interfaces(5) config file, e.g.
	// "up", "down", "mtu", etc.
	ExtraConfig map[string]string `json:",omitempty"`
}

// Port encapsulates a protocol and port number. It is used in API
// requests/responses. See also network.Port, from/to which this is
// transformed.
type Port struct {
	Protocol string `json:"Protocol"`
	Number   int    `json:"Number"`
}

// FromNetworkPort is a convenience helper to create a parameter
// out of the network type.
func FromNetworkPort(p network.Port) Port {
	return Port{
		Protocol: p.Protocol,
		Number:   p.Number,
	}
}

// NetworkPort is a convenience helper to return the parameter
// as network type.
func (p Port) NetworkPort() network.Port {
	return network.Port{
		Protocol: p.Protocol,
		Number:   p.Number,
	}
}

// PortRange represents a single range of ports. It is used in API
// requests/responses. See also network.PortRange, from/to which this is
// transformed.
type PortRange struct {
	FromPort int    `json:"FromPort"`
	ToPort   int    `json:"ToPort"`
	Protocol string `json:"Protocol"`
}

// FromNetworkPortRange is a convenience helper to create a parameter
// out of the network type.
func FromNetworkPortRange(pr network.PortRange) PortRange {
	return PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
}

// NetworkPortRange is a convenience helper to return the parameter
// as network type.
func (pr PortRange) NetworkPortRange() (network.PortRange, error) {
	npr := network.PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
	if err := npr.Validate(); err != nil {
		return network.PortRange{}, err
	}
	return npr, nil
}

// Address represents the location of a machine, including metadata
// about what kind of location the address describes. It's used in
// the API requests/responses. See also network.Address, from/to
// which this is transformed.
type Address struct {
	Value       string `json:"Value"`
	Type        string `json:"Type"`
	NetworkName string `json:"NetworkName"`
	Scope       string `json:"Scope"`
}

// FromNetworkAddress is a convenience helper to create a parameter
// out of the network type.
func FromNetworkAddress(addr network.Address) Address {
	return Address{
		Value:       addr.Value,
		Type:        string(addr.Type),
		NetworkName: addr.NetworkName,
		Scope:       string(addr.Scope),
	}
}

// NetworkAddress is a convenience helper to return the parameter
// as network type.
func (addr Address) NetworkAddress() network.Address {
	return network.Address{
		Value:       addr.Value,
		Type:        network.AddressType(addr.Type),
		NetworkName: addr.NetworkName,
		Scope:       network.Scope(addr.Scope),
	}
}

// HostPort associates an address with a port. It's used in
// the API requests/responses. See also network.HostPort, from/to
// which this is transformed.
type HostPort struct {
	Address
	Port int `json:"Port"`
}

// FromNetworkHostPort is a convenience helper to create a parameter
// out of the network type.
func FromNetworkHostPort(hp network.HostPort) HostPort {
	return HostPort{FromNetworkAddress(hp.Address), hp.Port}
}

// NetworkHostPort is a convenience helper to return the parameter
// as network type.
func (hp HostPort) NetworkHostPort() network.HostPort {
	return network.HostPort{hp.Address.NetworkAddress(), hp.Port}
}

// MachineAddresses holds an machine tag and addresses.
type MachineAddresses struct {
	Tag       string    `json:"Tag"`
	Addresses []Address `json:"Addresses"`
}

// MachinePortRange holds a single port range open on a machine for
// the given unit and relation tags.
type MachinePortRange struct {
	UnitTag     string    `json:"UnitTag"`
	RelationTag string    `json:"RelationTag"`
	PortRange   PortRange `json:"PortRange"`
}

// API request / response types.

// PortsResults holds the bulk operation result of an API call
// that returns a slice of Port.
type PortsResults struct {
	Results []PortsResult `json:"Results"`
}

// PortsResult holds the result of an API call that returns a slice
// of Port or an error.
type PortsResult struct {
	Error *Error `json:"Error"`
	Ports []Port `json:"Ports"`
}

// RequestedNetworkResult holds requested networks or an error.
type RequestedNetworkResult struct {
	Error    *Error   `json:"Error"`
	Networks []string `json:"Networks"`
}

// RequestedNetworksResults holds multiple requested networks results.
type RequestedNetworksResults struct {
	Results []RequestedNetworkResult `json:"Results"`
}

// MachineNetworkInfoResult holds network info for a single machine.
type MachineNetworkInfoResult struct {
	Error *Error        `json:"Error"`
	Info  []NetworkInfo `json:"Info"`
}

// MachineNetworkInfoResults holds network info for multiple machines.
type MachineNetworkInfoResults struct {
	Results []MachineNetworkInfoResult `json:"Results"`
}
