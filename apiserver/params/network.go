// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

// -----
// Parameters field types.
// -----

// Subnet describes a single subnet within a network.
type Subnet struct {
	// CIDR of the subnet in IPv4 or IPv6 notation.
	CIDR string `json:"cidr"`

	// ProviderId is the provider-specific subnet ID (if applicable).
	ProviderId string `json:"provider-id,omitempty"`

	// ProviderNetworkId is the id of the network containing this
	// subnet from the provider's perspective. It can be empty if the
	// provider doesn't support distinct networks.
	ProviderNetworkId string `json:"provider-network-id,omitempty"`

	// ProviderSpaceId is the id of the space containing this subnet
	// from the provider's perspective. It can be empty if the
	// provider doesn't support spaces (in which case all subnets are
	// effectively in the default space).
	ProviderSpaceId string `json:"provider-space-id,omitempty"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"vlan-tag"`

	// Life is the subnet's life cycle value - Alive means the subnet
	// is in use by one or more machines, Dying or Dead means the
	// subnet is about to be removed.
	Life Life `json:"life"`

	// SpaceTag is the Juju network space this subnet is associated
	// with.
	SpaceTag string `json:"space-tag"`

	// Zones contain one or more availability zones this subnet is
	// associated with.
	Zones []string `json:"zones"`

	// Status returns the status of the subnet, whether it is in use, not
	// in use or terminating.
	Status string `json:"status,omitempty"`
}

// NetworkRoute describes a special route that should be added for a given
// network interface.
type NetworkRoute struct {
	// DestinationCIDR is the Subnet CIDR of traffic that needs a custom route.
	DestinationCIDR string `json:"destination-cidr"`
	// GatewayIP is the target IP to use as the next-hop when sending traffic to DestinationCIDR
	GatewayIP string `json:"gateway-ip"`
	// Metric is the cost for this particular route.
	Metric int `json:"metric"`
}

// NetworkConfig describes the necessary information to configure
// a single network interface on a machine. This mostly duplicates
// network.InterfaceInfo type and it's defined here so it can be kept
// separate and stable as definition to ensure proper wire-format for
// the API.
type NetworkConfig struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int `json:"device-index"`

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string `json:"mac-address"`

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string `json:"cidr"`

	// MTU is the Maximum Transmission Unit controlling the maximum size of the
	// protocol packets that the interface can pass through. It is only used
	// when > 0.
	MTU int `json:"mtu"`

	// ProviderId is a provider-specific network interface id.
	ProviderId string `json:"provider-id"`

	// ProviderSubnetId is a provider-specific subnet id, to which the
	// interface is attached to.
	ProviderSubnetId string `json:"provider-subnet-id"`

	// ProviderSpaceId is a provider-specific space id, to which the interface
	// is attached to, if known and supported.
	ProviderSpaceId string `json:"provider-space-id"`

	// ProviderAddressId is the provider-specific id of the assigned address, if
	// supported and known.
	ProviderAddressId string `json:"provider-address-id"`

	// ProviderVLANId is the provider-specific id of the assigned address's
	// VLAN, if supported and known.
	ProviderVLANId string `json:"provider-vlan-id"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"vlan-tag"`

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string `json:"interface-name"`

	// ParentInterfaceName is the name of the parent interface to use, if known.
	ParentInterfaceName string `json:"parent-interface-name"`

	// InterfaceType is the type of the interface.
	InterfaceType string `json:"interface-type"`

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it at all or stop it if running.
	Disabled bool `json:"disabled"`

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot. By default and for
	// backwards-compatibility, interfaces are configured to
	// auto-start.
	NoAutoStart bool `json:"no-auto-start,omitempty"`

	// ConfigType, if set, defines what type of configuration to use.
	// See network.InterfaceConfigType for more info. If not set, for
	// backwards-compatibility, "dhcp" is assumed.
	ConfigType string `json:"config-type,omitempty"`

	// Address contains an optional static IP address to configure for
	// this network interface. The subnet mask to set will be inferred
	// from the CIDR value.
	Address string `json:"address,omitempty"`

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers []string `json:"dns-servers,omitempty"`

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSSearchDomains []string `json:"dns-search-domains,omitempty"`

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually (one of) the host address(es).
	GatewayAddress string `json:"gateway-address,omitempty"`

	// Routes is a list of routes that should be applied when this interface is
	// active.
	Routes []NetworkRoute `json:"routes,omitempty"`

	// IsDefaultGateway marks an interface that is a default gateway for a machine.
	IsDefaultGateway bool `json:"is-default-gateway,omitempty"`
}

// DeviceBridgeInfo lists the host device and the expected bridge to be
// created.
type DeviceBridgeInfo struct {
	HostDeviceName string `json:"host-device-name"`
	BridgeName     string `json:"bridge-name"`
	MACAddress     string `json:"mac-address"`
}

// ProviderInterfaceInfoResults holds the results of a
// GetProviderInterfaceInfo call.
type ProviderInterfaceInfoResults struct {
	Results []ProviderInterfaceInfoResult `json:"results"`
}

// ProviderInterfaceInfoResult stores the provider interface
// information for one machine, or any error that occurred getting the
// information for that machine.
type ProviderInterfaceInfoResult struct {
	MachineTag string                  `json:"machine-tag"`
	Interfaces []ProviderInterfaceInfo `json:"interfaces"`
	Error      *Error                  `json:"error,omitempty"`
}

// ProviderInterfaceInfo stores the details needed to identify an
// interface to a provider. It's the params equivalent of
// network.ProviderInterfaceInfo, defined here separately to ensure
// that API structures aren't inadvertently changed by internal
// changes.
type ProviderInterfaceInfo struct {
	InterfaceName string `json:"interface-name"`
	MACAddress    string `json:"mac-address"`
	ProviderId    string `json:"provider-id"`
}

// Port encapsulates a protocol and port number. It is used in API
// requests/responses. See also network.Port, from/to which this is
// transformed.
type Port struct {
	Protocol string `json:"protocol"`
	Number   int    `json:"number"`
}

// FromNetworkPort is a convenience helper to create a parameter
// out of the network type, here for Port.
func FromNetworkPort(p corenetwork.Port) Port {
	return Port{
		Protocol: p.Protocol,
		Number:   p.Number,
	}
}

// NetworkPort is a convenience helper to return the parameter
// as network type, here for Port.
func (p Port) NetworkPort() corenetwork.Port {
	return corenetwork.Port{
		Protocol: p.Protocol,
		Number:   p.Number,
	}
}

// PortRange represents a single range of ports. It is used in API
// requests/responses. See also network.PortRange, from/to which this is
// transformed.
type PortRange struct {
	FromPort int    `json:"from-port"`
	ToPort   int    `json:"to-port"`
	Protocol string `json:"protocol"`
}

// FromNetworkPortRange is a convenience helper to create a parameter
// out of the network type, here for PortRange.
func FromNetworkPortRange(pr corenetwork.PortRange) PortRange {
	return PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
}

// NetworkPortRange is a convenience helper to return the parameter
// as network type, here for PortRange.
func (pr PortRange) NetworkPortRange() corenetwork.PortRange {
	return corenetwork.PortRange{
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
		Protocol: pr.Protocol,
	}
}

// EntityPort holds an entity's tag, a protocol and a port.
type EntityPort struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// EntitiesPorts holds the parameters for making an OpenPort or
// ClosePort on some entities.
type EntitiesPorts struct {
	Entities []EntityPort `json:"entities"`
}

// EntityPortRange holds an entity's tag, a protocol and a port range.
type EntityPortRange struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	FromPort int    `json:"from-port"`
	ToPort   int    `json:"to-port"`
}

// EntitiesPortRanges holds the parameters for making an OpenPorts or
// ClosePorts on some entities.
type EntitiesPortRanges struct {
	Entities []EntityPortRange `json:"entities"`
}

// Address represents the location of a machine, including metadata
// about what kind of location the address describes. It's used in
// the API requests/responses. See also network.Address, from/to
// which this is transformed.
type Address struct {
	Value           string `json:"value"`
	Type            string `json:"type"`
	Scope           string `json:"scope"`
	SpaceName       string `json:"space-name,omitempty"`
	SpaceProviderId string `json:"space-id,omitempty"`
}

// FromNetworkAddress is a convenience helper to create a parameter
// out of the network type, here for Address.
func FromNetworkAddress(naddr network.Address) Address {
	return Address{
		Value:           naddr.Value,
		Type:            string(naddr.Type),
		Scope:           string(naddr.Scope),
		SpaceName:       string(naddr.SpaceName),
		SpaceProviderId: string(naddr.SpaceProviderId),
	}
}

// NetworkAddress is a convenience helper to return the parameter
// as network type, here for Address.
func (addr Address) NetworkAddress() network.Address {
	return network.Address{
		Value:           addr.Value,
		Type:            network.AddressType(addr.Type),
		Scope:           network.Scope(addr.Scope),
		SpaceName:       network.SpaceName(addr.SpaceName),
		SpaceProviderId: corenetwork.Id(addr.SpaceProviderId),
	}
}

// FromNetworkAddresses is a convenience helper to create a parameter
// out of the network type, here for a slice of Address.
func FromNetworkAddresses(naddrs ...network.Address) []Address {
	addrs := make([]Address, len(naddrs))
	for i, naddr := range naddrs {
		addrs[i] = FromNetworkAddress(naddr)
	}
	return addrs
}

// NetworkAddresses is a convenience helper to return the parameter
// as network type, here for a slice of Address.
func NetworkAddresses(addrs ...Address) []network.Address {
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
	Port int `json:"port"`
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

// TODO (wpk) Uniter.NetworkConfig API is obsolete, use NetworkInfo instead
// UnitsNetworkConfig holds the parameters for calling Uniter.NetworkConfig()
// API. We need to retain until V4 of the Uniter API is removed.
type UnitsNetworkConfig struct {
	Args []UnitNetworkConfig `json:"args"`
}

// UnitNetworkConfig holds a unit tag and an endpoint binding name.
type UnitNetworkConfig struct {
	UnitTag     string `json:"unit-tag"`
	BindingName string `json:"binding-name"`
}

// MachineAddresses holds an machine tag and addresses.
type MachineAddresses struct {
	Tag       string    `json:"tag"`
	Addresses []Address `json:"addresses"`
}

// SetMachinesAddresses holds the parameters for making an
// API call to update machine addresses.
type SetMachinesAddresses struct {
	MachineAddresses []MachineAddresses `json:"machine-addresses"`
}

// SetMachineNetworkConfig holds the parameters for making an API call to update
// machine network config.
type SetMachineNetworkConfig struct {
	Tag    string          `json:"tag"`
	Config []NetworkConfig `json:"config"`
}

// MachineAddressesResult holds a list of machine addresses or an
// error.
type MachineAddressesResult struct {
	Error     *Error    `json:"error,omitempty"`
	Addresses []Address `json:"addresses"`
}

// MachineAddressesResults holds the results of calling an API method
// returning a list of addresses per machine.
type MachineAddressesResults struct {
	Results []MachineAddressesResult `json:"results"`
}

// MachinePortRange holds a single port range open on a machine for
// the given unit and relation tags.
type MachinePortRange struct {
	UnitTag     string    `json:"unit-tag"`
	RelationTag string    `json:"relation-tag"`
	PortRange   PortRange `json:"port-range"`
}

// MachinePorts holds a machine and subnet tags. It's used when referring to
// opened ports on the machine for a subnet.
type MachinePorts struct {
	MachineTag string `json:"machine-tag"`
	SubnetTag  string `json:"subnet-tag"`
}

// -----
// API request / response types.
// -----

// PortsResults holds the bulk operation result of an API call
// that returns a slice of Port.
type PortsResults struct {
	Results []PortsResult `json:"results"`
}

// PortsResult holds the result of an API call that returns a slice
// of Port or an error.
type PortsResult struct {
	Error *Error `json:"error,omitempty"`
	Ports []Port `json:"ports"`
}

// UnitNetworkConfigResult holds network configuration for a single unit.
type UnitNetworkConfigResult struct {
	Error *Error `json:"error,omitempty"`

	// Tagged to Info due to compatibility reasons.
	Config []NetworkConfig `json:"info"`
}

// UnitNetworkConfigResults holds network configuration for multiple units.
type UnitNetworkConfigResults struct {
	Results []UnitNetworkConfigResult `json:"results"`
}

// MachineNetworkConfigResult holds network configuration for a single machine.
type MachineNetworkConfigResult struct {
	Error *Error `json:"error,omitempty"`

	// Tagged to Info due to compatibility reasons.
	Config []NetworkConfig `json:"info"`
}

// MachineNetworkConfigResults holds network configuration for multiple machines.
type MachineNetworkConfigResults struct {
	Results []MachineNetworkConfigResult `json:"results"`
}

// HostNetworkChange holds the information about how a host machine should be
// modified to prepare for a container.
type HostNetworkChange struct {
	Error *Error `json:"error,omitempty"`

	// NewBridges lists the bridges that need to be created and what host
	// device they should be connected to.
	NewBridges []DeviceBridgeInfo `json:"new-bridges"`

	// ReconfigureDelay is the duration in seconds to sleep before
	// raising the bridged interface
	ReconfigureDelay int `json:"reconfigure-delay"`
}

// HostNetworkChangeResults holds the network changes that are necessary for multiple containers to be created.
type HostNetworkChangeResults struct {
	Results []HostNetworkChange `json:"results"`
}

// MachinePortsParams holds the arguments for making a
// FirewallerAPIV1.GetMachinePorts() API call.
type MachinePortsParams struct {
	Params []MachinePorts `json:"params"`
}

// MachinePortsResult holds a single result of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResult struct {
	Error *Error             `json:"error,omitempty"`
	Ports []MachinePortRange `json:"ports"`
}

// MachinePortsResults holds all the results of the
// FirewallerAPIV1.GetMachinePorts() and UniterAPI.AllMachinePorts()
// API calls.
type MachinePortsResults struct {
	Results []MachinePortsResult `json:"results"`
}

// APIHostPortsResult holds the result of an APIHostPorts
// call. Each element in the top level slice holds
// the addresses for one API server.
type APIHostPortsResult struct {
	Servers [][]HostPort `json:"servers"`
}

// NetworkHostsPorts is a convenience helper to return the contained
// result servers as network type.
func (r APIHostPortsResult) NetworkHostsPorts() [][]network.HostPort {
	return NetworkHostsPorts(r.Servers)
}

// ZoneResult holds the result of an API call that returns an
// availability zone name and whether it's available for use.
type ZoneResult struct {
	Error     *Error `json:"error,omitempty"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// ZoneResults holds multiple ZoneResult results
type ZoneResults struct {
	Results []ZoneResult `json:"results"`
}

// SpaceResult holds a single space tag or an error.
type SpaceResult struct {
	Error *Error `json:"error,omitempty"`
	Tag   string `json:"tag"`
}

// SpaceResults holds the bulk operation result of an API call
// that returns space tags or an errors.
type SpaceResults struct {
	Results []SpaceResult `json:"results"`
}

// ListSubnetsResults holds the result of a ListSubnets API call.
type ListSubnetsResults struct {
	Results []Subnet `json:"results"`
}

// SubnetsFilters holds an optional SpaceTag and Zone for filtering
// the subnets returned by a ListSubnets call.
type SubnetsFilters struct {
	SpaceTag string `json:"space-tag,omitempty"`
	Zone     string `json:"zone,omitempty"`
}

// AddSubnetsParams holds the arguments of AddSubnets API call.
type AddSubnetsParams struct {
	Subnets []AddSubnetParams `json:"subnets"`
}

// AddSubnetParams holds a subnet and space tags, subnet provider ID,
// and a list of zones to associate the subnet to. Either SubnetTag or
// SubnetProviderId must be set, but not both. Zones can be empty if
// they can be discovered
type AddSubnetParams struct {
	SubnetTag         string   `json:"subnet-tag,omitempty"`
	SubnetProviderId  string   `json:"subnet-provider-id,omitempty"`
	ProviderNetworkId string   `json:"provider-network-id,omitempty"`
	SpaceTag          string   `json:"space-tag"`
	VLANTag           int      `json:"vlan-tag,omitempty"`
	Zones             []string `json:"zones,omitempty"`
}

// CreateSubnetsParams holds the arguments of CreateSubnets API call.
type CreateSubnetsParams struct {
	Subnets []CreateSubnetParams `json:"subnets"`
}

// CreateSubnetParams holds a subnet and space tags, vlan tag,
// and a list of zones to associate the subnet to.
type CreateSubnetParams struct {
	SubnetTag string   `json:"subnet-tag,omitempty"`
	SpaceTag  string   `json:"space-tag"`
	Zones     []string `json:"zones,omitempty"`
	VLANTag   int      `json:"vlan-tag,omitempty"`
	IsPublic  bool     `json:"is-public"`
}

// CreateSpacesParams olds the arguments of the AddSpaces API call.
type CreateSpacesParams struct {
	Spaces []CreateSpaceParams `json:"spaces"`
}

// CreateSpaceParams holds the space tag and at least one subnet
// tag required to create a new space.
type CreateSpaceParams struct {
	SubnetTags []string `json:"subnet-tags"`
	SpaceTag   string   `json:"space-tag"`
	Public     bool     `json:"public"`
	ProviderId string   `json:"provider-id,omitempty"`
}

// ListSpacesResults holds the list of all available spaces.
type ListSpacesResults struct {
	Results []Space `json:"results"`
}

// Space holds the information about a single space and its associated subnets.
type Space struct {
	Name    string   `json:"name"`
	Subnets []Subnet `json:"subnets"`
	Error   *Error   `json:"error,omitempty"`
}

// ProviderSpace holds the information about a single space and its associated subnets.
type ProviderSpace struct {
	Name       string   `json:"name"`
	ProviderId string   `json:"provider-id"`
	Subnets    []Subnet `json:"subnets"`
	Error      *Error   `json:"error,omitempty"`
}

type ProxyConfig struct {
	HTTP    string `json:"http"`
	HTTPS   string `json:"https"`
	FTP     string `json:"ftp"`
	NoProxy string `json:"no-proxy"`
}

// ProxyConfigResult contains information needed to configure a clients proxy settings
type ProxyConfigResult struct {
	LegacyProxySettings      ProxyConfig `json:"legacy-proxy-settings"`
	JujuProxySettings        ProxyConfig `json:"juju-proxy-settings"`
	APTProxySettings         ProxyConfig `json:"apt-proxy-settings,omitempty"`
	SnapProxySettings        ProxyConfig `json:"snap-proxy-settings,omitempty"`
	SnapStoreProxyId         string      `json:"snap-store-id,omitempty"`
	SnapStoreProxyAssertions string      `json:"snap-store-assertions,omitempty"`
	Error                    *Error      `json:"error,omitempty"`
}

// ProxyConfigResults contains information needed to configure multiple clients proxy settings
type ProxyConfigResults struct {
	Results []ProxyConfigResult `json:"results"`
}

// ProxyConfigResultV1 contains information needed to configure a clients proxy settings.
// Result for facade v1 call.
type ProxyConfigResultV1 struct {
	ProxySettings    ProxyConfig `json:"proxy-settings"`
	APTProxySettings ProxyConfig `json:"apt-proxy-settings"`
	Error            *Error      `json:"error,omitempty"`
}

// ProxyConfigResultsV1 contains information needed to configure multiple clients proxy settings.
// Result for facade v1 call.
type ProxyConfigResultsV1 struct {
	Results []ProxyConfigResultV1 `json:"results"`
}

// InterfaceAddress represents a single address attached to the interface.
type InterfaceAddress struct {
	Hostname string `json:"hostname"`
	Address  string `json:"value"`
	CIDR     string `json:"cidr"`
}

// NetworkInfo describes one interface with IP addresses.
type NetworkInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string `json:"mac-address"`

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string `json:"interface-name"`

	// Addresses contains a list of addresses configured on the interface.
	Addresses []InterfaceAddress `json:"addresses"`
}

// NetworkInfoResult Adds egress and ingress subnets and changes the serialized
// `Info` key name in the yaml/json API protocol.
type NetworkInfoResult struct {
	Error            *Error        `json:"error,omitempty" yaml:"error,omitempty"`
	Info             []NetworkInfo `json:"bind-addresses,omitempty" yaml:"bind-addresses,omitempty"`
	EgressSubnets    []string      `json:"egress-subnets,omitempty" yaml:"egress-subnets,omitempty"`
	IngressAddresses []string      `json:"ingress-addresses,omitempty" yaml:"ingress-addresses,omitempty"`
}

// NetworkInfoResults holds a mapping from binding name to NetworkInfoResult.
type NetworkInfoResults struct {
	Results map[string]NetworkInfoResult `json:"results"`
}

// NetworkInfoResultV6 holds either and error or a list of NetworkInfos for given binding.
type NetworkInfoResultV6 struct {
	Error *Error        `json:"error,omitempty" yaml:"error,omitempty"`
	Info  []NetworkInfo `json:"network-info" yaml:"info"`
}

// NetworkInfoResults holds a mapping from binding name to NetworkInfoResultV6.
type NetworkInfoResultsV6 struct {
	Results map[string]NetworkInfoResultV6 `json:"results"`
}

// NetworkInfoParams holds a name of the unit and list of bindings for which we want to get NetworkInfos.
type NetworkInfoParams struct {
	Unit       string   `json:"unit"`
	RelationId *int     `json:"relation-id,omitempty"`
	Bindings   []string `json:"bindings"`
}

// FanConfigEntry holds configuration for a single fan.
type FanConfigEntry struct {
	Underlay string `json:"underlay"`
	Overlay  string `json:"overlay"`
}

// FanConfigResult holds configuration for all fans in a model
type FanConfigResult struct {
	Fans []FanConfigEntry `json:"fans"`
}
