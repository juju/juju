// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"net"

	"github.com/juju/juju/network"
)

// -----
// Parameters field types.
// -----

// Subnet describes a single subnet within a network.
type Subnet struct {
	// CIDR of the subnet in IPv4 or IPv6 notation.
	CIDR string `json:"CIDR"`

	// ProviderId is the provider-specific subnet ID (if applicable).
	ProviderId string `json:"ProviderId,omitempty`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"VLANTag"`

	// Life is the subnet's life cycle value - Alive means the subnet
	// is in use by one or more machines, Dying or Dead means the
	// subnet is about to be removed.
	Life Life `json:"Life"`

	// SpaceTag is the Juju network space this subnet is associated
	// with.
	SpaceTag string `json:"SpaceTag"`

	// Zones contain one or more availability zones this subnet is
	// associated with.
	Zones []string `json:"Zones"`

	// StaticRangeLowIP (if available) is the lower bound of the
	// subnet's static IP allocation range.
	StaticRangeLowIP net.IP `json:"StaticRangeLowIP,omitempty"`

	// StaticRangeHighIP (if available) is the higher bound of the
	// subnet's static IP allocation range.
	StaticRangeHighIP net.IP `json:"StaticRangeHighIP,omitempty"`

	// Status returns the status of the subnet, whether it is in use, not
	// in use or terminating.
	Status string `json:"Status,omitempty"`
}

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

// NetworkConfig describes the necessary information to configure
// a single network interface on a machine. This mostly duplicates
// network.InterfaceInfo type and it's defined here so it can be kept
// separate and stable as definition to ensure proper wire-format for
// the API.
type NetworkConfig struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int `json:"DeviceIndex"`

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string `json:"MACAddress"`

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string `json:"CIDR"`

	// NetworkName is juju-internal name of the network.
	// TODO(dimitern) This should be removed or adapted to the model
	// once spaces are introduced.
	NetworkName string `json:"NetworkName"`

	// ProviderId is a provider-specific network interface id.
	ProviderId string `json:"ProviderId"`

	// ProviderSubnetId is a provider-specific subnet id, to which the
	// interface is attached to.
	ProviderSubnetId string `json:"ProviderSubnetId"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"VLANTag"`

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string `json:"InterfaceName"`

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it at all or stop it if running.
	Disabled bool `json:"Disabled"`

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot. By default and for
	// backwards-compatibility, interfaces are configured to
	// auto-start.
	NoAutoStart bool `json:"NoAutoStart,omitempty"`

	// ConfigType, if set, defines what type of configuration to use.
	// See network.InterfaceConfigType for more info. If not set, for
	// backwards-compatibility, "dhcp" is assumed.
	ConfigType string `json:"ConfigType,omitempty"`

	// Address contains an optional static IP address to configure for
	// this network interface. The subnet mask to set will be inferred
	// from the CIDR value.
	Address string `json:"Address,omitempty"`

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers []string `json:"DNSServers,omitempty"`

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually (one of) the host address(es).
	GatewayAddress string `json:"GatewayAddress,omitempty"`

	// ExtraConfig can contain any valid setting and its value allowed
	// inside an "iface" section of a interfaces(5) config file, e.g.
	// "up", "down", "mtu", etc.
	ExtraConfig map[string]string `json:"ExtraConfig,omitempty"`
}

// Port encapsulates a protocol and port number. It is used in API
// requests/responses. See also network.Port, from/to which this is
// transformed.
type Port struct {
	Protocol string `json:"Protocol"`
	Number   int    `json:"Number"`
}

// FromNetworkPort is a convenience helper to create a parameter
// out of the network type, here for Port.
func FromNetworkPort(p network.Port) Port {
	return Port{
		Protocol: p.Protocol,
		Number:   p.Number,
	}
}

// NetworkPort is a convenience helper to return the parameter
// as network type, here for Port.
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
// out of the network type, here for PortRange.
func FromNetworkPortRange(pr network.PortRange) PortRange {
	return PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
}

// NetworkPortRange is a convenience helper to return the parameter
// as network type, here for PortRange.
func (pr PortRange) NetworkPortRange() network.PortRange {
	return network.PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
}

// EntityPort holds an entity's tag, a protocol and a port.
type EntityPort struct {
	Tag      string `json:"Tag"`
	Protocol string `json:"Protocol"`
	Port     int    `json:"Port"`
}

// EntitiesPorts holds the parameters for making an OpenPort or
// ClosePort on some entities.
type EntitiesPorts struct {
	Entities []EntityPort `json:"Entities"`
}

// EntityPortRange holds an entity's tag, a protocol and a port range.
type EntityPortRange struct {
	Tag      string `json:"Tag"`
	Protocol string `json:"Protocol"`
	FromPort int    `json:"FromPort"`
	ToPort   int    `json:"ToPort"`
}

// EntitiesPortRanges holds the parameters for making an OpenPorts or
// ClosePorts on some entities.
type EntitiesPortRanges struct {
	Entities []EntityPortRange `json:"Entities"`
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
// out of the network type, here for Address.
func FromNetworkAddress(naddr network.Address) Address {
	return Address{
		Value:       naddr.Value,
		Type:        string(naddr.Type),
		NetworkName: naddr.NetworkName,
		Scope:       string(naddr.Scope),
	}
}

// NetworkAddress is a convenience helper to return the parameter
// as network type, here for Address.
func (addr Address) NetworkAddress() network.Address {
	return network.Address{
		Value:       addr.Value,
		Type:        network.AddressType(addr.Type),
		NetworkName: addr.NetworkName,
		Scope:       network.Scope(addr.Scope),
	}
}

// FromNetworkAddresses is a convenience helper to create a parameter
// out of the network type, here for a slice of Address.
func FromNetworkAddresses(naddrs []network.Address) []Address {
	addrs := make([]Address, len(naddrs))
	for i, naddr := range naddrs {
		addrs[i] = FromNetworkAddress(naddr)
	}
	return addrs
}

// NetworkAddresses is a convenience helper to return the parameter
// as network type, here for a slice of Address.
func NetworkAddresses(addrs []Address) []network.Address {
	naddrs := make([]network.Address, len(addrs))
	for i, addr := range addrs {
		naddrs[i] = addr.NetworkAddress()
	}
	return naddrs
}

// HostPort associates an address with a port. It's used in
// the API requests/responses. See also network.HostPort, from/to
// which this is transformed.
type HostPort struct {
	Address
	Port int `json:"Port"`
}

// FromNetworkHostPort is a convenience helper to create a parameter
// out of the network type, here for HostPort.
func FromNetworkHostPort(nhp network.HostPort) HostPort {
	return HostPort{FromNetworkAddress(nhp.Address), nhp.Port}
}

// NetworkHostPort is a convenience helper to return the parameter
// as network type, here for HostPort.
func (hp HostPort) NetworkHostPort() network.HostPort {
	return network.HostPort{hp.Address.NetworkAddress(), hp.Port}
}

// FromNetworkHostPorts is a helper to create a parameter
// out of the network type, here for a slice of HostPort.
func FromNetworkHostPorts(nhps []network.HostPort) []HostPort {
	hps := make([]HostPort, len(nhps))
	for i, nhp := range nhps {
		hps[i] = FromNetworkHostPort(nhp)
	}
	return hps
}

// NetworkHostPorts is a convenience helper to return the parameter
// as network type, here for a slice of HostPort.
func NetworkHostPorts(hps []HostPort) []network.HostPort {
	nhps := make([]network.HostPort, len(hps))
	for i, hp := range hps {
		nhps[i] = hp.NetworkHostPort()
	}
	return nhps
}

// FromNetworkHostsPorts is a helper to create a parameter
// out of the network type, here for a nested slice of HostPort.
func FromNetworkHostsPorts(nhpm [][]network.HostPort) [][]HostPort {
	hpm := make([][]HostPort, len(nhpm))
	for i, nhps := range nhpm {
		hpm[i] = FromNetworkHostPorts(nhps)
	}
	return hpm
}

// NetworkHostsPorts is a convenience helper to return the parameter
// as network type, here for a nested slice of HostPort.
func NetworkHostsPorts(hpm [][]HostPort) [][]network.HostPort {
	nhpm := make([][]network.HostPort, len(hpm))
	for i, hps := range hpm {
		nhpm[i] = NetworkHostPorts(hps)
	}
	return nhpm
}

// MachineAddresses holds an machine tag and addresses.
type MachineAddresses struct {
	Tag       string    `json:"Tag"`
	Addresses []Address `json:"Addresses"`
}

// SetMachinesAddresses holds the parameters for making an
// API call to update machine addresses.
type SetMachinesAddresses struct {
	MachineAddresses []MachineAddresses `json:"MachineAddresses"`
}

// MachineAddressesResult holds a list of machine addresses or an
// error.
type MachineAddressesResult struct {
	Error     *Error    `json:"Error"`
	Addresses []Address `json:"Addresses"`
}

// MachineAddressesResults holds the results of calling an API method
// returning a list of addresses per machine.
type MachineAddressesResults struct {
	Results []MachineAddressesResult `json:"Results"`
}

// MachinePortRange holds a single port range open on a machine for
// the given unit and relation tags.
type MachinePortRange struct {
	UnitTag     string    `json:"UnitTag"`
	RelationTag string    `json:"RelationTag"`
	PortRange   PortRange `json:"PortRange"`
}

// MachinePorts holds a machine and network tags. It's used when
// referring to opened ports on the machine for a network.
type MachinePorts struct {
	MachineTag string `json:"MachineTag"`
	NetworkTag string `json:"NetworkTag"`
}

// -----
// API request / response types.
// -----

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

// MachineNetworkConfigResult holds network configuration for a single machine.
type MachineNetworkConfigResult struct {
	Error *Error `json:"Error"`

	// Tagged to Info due to compatibility reasons.
	Config []NetworkConfig `json:"Info"`
}

// MachineNetworkConfigResults holds network configuration for multiple machines.
type MachineNetworkConfigResults struct {
	Results []MachineNetworkConfigResult `json:"Results"`
}

// MachinePortsParams holds the arguments for making a
// FirewallerAPIV1.GetMachinePorts() API call.
type MachinePortsParams struct {
	Params []MachinePorts `json:"Params"`
}

// MachinePortsResult holds a single result of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResult struct {
	Error *Error             `json:"Error"`
	Ports []MachinePortRange `json:"Ports"`
}

// MachinePortsResults holds all the results of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResults struct {
	Results []MachinePortsResult `json:"Results"`
}

// APIHostPortsResult holds the result of an APIHostPorts
// call. Each element in the top level slice holds
// the addresses for one API server.
type APIHostPortsResult struct {
	Servers [][]HostPort `json:"Servers"`
}

// NetworkHostsPorts is a convenience helper to return the contained
// result servers as network type.
func (r APIHostPortsResult) NetworkHostsPorts() [][]network.HostPort {
	return NetworkHostsPorts(r.Servers)
}

// ZoneResult holds the result of an API call that returns an
// availability zone name and whether it's available for use.
type ZoneResult struct {
	Error     *Error `json:"Error"`
	Name      string `json:"Name"`
	Available bool   `json:"Available"`
}

// ZoneResults holds multiple ZoneResult results
type ZoneResults struct {
	Results []ZoneResult `json:"Results"`
}

// SpaceResult holds a single space tag or an error.
type SpaceResult struct {
	Error *Error `json:"Error"`
	Tag   string `json:"Tag"`
}

// SpaceResults holds the bulk operation result of an API call
// that returns space tags or an errors.
type SpaceResults struct {
	Results []SpaceResult `json:"Results"`
}

// ListSubnetsResults holds the result of a ListSubnets API call.
type ListSubnetsResults struct {
	Results []Subnet `json:"Results"`
}

// SubnetsFilters holds an optional SpaceTag and Zone for filtering
// the subnets returned by a ListSubnets call.
type SubnetsFilters struct {
	SpaceTag string `json:"SpaceTag,omitempty"`
	Zone     string `json:"Zone,omitempty"`
}

// AddSubnetsParams holds the arguments of AddSubnets API call.
type AddSubnetsParams struct {
	Subnets []AddSubnetParams `json:"Subnets"`
}

// AddSubnetParams holds a subnet and space tags, subnet provider ID,
// and a list of zones to associate the subnet to. Either SubnetTag or
// SubnetProviderId must be set, but not both. Zones can be empty if
// they can be discovered
type AddSubnetParams struct {
	SubnetTag        string   `json:"SubnetTag,omitempty"`
	SubnetProviderId string   `json:"SubnetProviderId,omitempty"`
	SpaceTag         string   `json:"SpaceTag"`
	Zones            []string `json:"Zones,omitempty"`
}

// CreateSubnetsParams holds the arguments of CreateSubnets API call.
type CreateSubnetsParams struct {
	Subnets []CreateSubnetParams `json:"Subnets"`
}

// CreateSubnetParams holds a subnet and space tags, vlan tag,
// and a list of zones to associate the subnet to.
type CreateSubnetParams struct {
	SubnetTag string   `json:"SubnetTag,omitempty"`
	SpaceTag  string   `json:"SpaceTag"`
	Zones     []string `json:"Zones,omitempty"`
	VLANTag   int      `json:"VLANTag,omitempty"`
	IsPublic  bool     `json:"IsPublic"`
}

// CreateSpacesParams olds the arguments of the AddSpaces API call.
type CreateSpacesParams struct {
	Spaces []CreateSpaceParams `json:"Spaces"`
}

// CreateSpaceParams holds the space tag and at least one subnet
// tag required to create a new space.
type CreateSpaceParams struct {
	SubnetTags []string `json:"SubnetTags"`
	SpaceTag   string   `json:"SpaceTag"`
	Public     bool     `json:"Public"`
}

// ListSpacesResults holds the list of all available spaces.
type ListSpacesResults struct {
	Results []Space `json:"Results"`
}

// Space holds the information about a single space and its associated subnets.
type Space struct {
	Name    string   `json:"Name"`
	Subnets []Subnet `json:"Subnets"`
	Error   *Error   `json:"Error,omitempty"`
}
