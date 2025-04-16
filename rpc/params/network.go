// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
)

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
	Life life.Value `json:"life"`

	// SpaceTag is the Juju network space this subnet is associated
	// with.
	SpaceTag string `json:"space-tag"`

	// Zones contain one or more availability zones this subnet is
	// associated with.
	Zones []string `json:"zones"`

	// TODO (jack-w-shaw 2022-02-22): Remove this. It is unused
	//
	// Status returns the status of the subnet, whether it is in use, not
	// in use or terminating.
	Status string `json:"status,omitempty"`
}

// SubnetV2 is used by versions of spaces/subnets APIs that must include
// subnet ID in payloads.
type SubnetV2 struct {
	Subnet

	// ID uniquely identifies the subnet.
	ID string `json:"id,omitempty"`
}

// SubnetV3 is used by the SpaceInfos API call. Its payload matches the fields
// of the core/network.SubnetInfo struct.
type SubnetV3 struct {
	SubnetV2

	SpaceID  string `json:"space-id"`
	IsPublic bool   `json:"is-public,omitempty"`
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

// NetworkOrigin specifies where an address comes from, whether it was reported
// by a provider or by a machine.
type NetworkOrigin string

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

	// ProviderNetworkId is a provider-specific id for the network this
	// interface is part of.
	ProviderNetworkId string `json:"provider-network-id"`

	// ProviderSubnetId is a provider-specific subnet id, to which the
	// interface is attached to.
	ProviderSubnetId string `json:"provider-subnet-id"`

	// ProviderSpaceId is a provider-specific space id to which the interface
	// is attached, if known and supported.
	ProviderSpaceId string `json:"provider-space-id"`

	// ProviderAddressId is the provider-specific id of the assigned address,
	// if supported and known.
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
	// See network.AddressConfigType for more info. If not set, for
	// backwards-compatibility, "dhcp" is assumed.
	ConfigType string `json:"config-type,omitempty"`

	// Addresses contains an optional list of static IP address to
	// configure for this network interface. The subnet mask to set will be
	// inferred from the CIDR value of the first entry which is always
	// assumed to be the primary IP address for the interface.
	Addresses []Address `json:"addresses,omitempty"`

	// ShadowAddresses contains an optional list of additional IP addresses
	// that the underlying network provider associates with this network
	// interface instance. These IP addresses are not typically visible
	// to the machine that the interface is connected to.
	ShadowAddresses []Address `json:"shadow-addresses,omitempty"`

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

	// VirtualPortType provides additional information about the type of
	// this device if it belongs to a virtual switch (e.g. when using
	// open-vswitch).
	VirtualPortType string `json:"virtual-port-type,omitempty"`

	// NetworkOrigin represents the authoritative source of the NetworkConfig.
	// It is expected that either the provider gave us this info or the
	// machine gave us this info.
	// Giving us this information allows us to reason about when a InterfaceInfo
	// is in use.
	NetworkOrigin NetworkOrigin `json:"origin,omitempty"`
}

// NetworkConfigFromInterfaceInfo converts a slice of network.InterfaceInfo into
// the equivalent NetworkConfig slice.
func NetworkConfigFromInterfaceInfo(interfaceInfos network.InterfaceInfos) []NetworkConfig {
	result := make([]NetworkConfig, len(interfaceInfos))
	for i, v := range interfaceInfos {
		var dnsServers []string
		for _, nameserver := range v.DNSServers {
			dnsServers = append(dnsServers, nameserver.Value)
		}

		var routes []NetworkRoute
		if len(v.Routes) != 0 {
			routes = make([]NetworkRoute, len(v.Routes))
			for j, route := range v.Routes {
				routes[j] = NetworkRoute{
					DestinationCIDR: route.DestinationCIDR,
					GatewayIP:       route.GatewayIP,
					Metric:          route.Metric,
				}
			}
		}

		result[i] = NetworkConfig{
			DeviceIndex:         v.DeviceIndex,
			MACAddress:          network.NormalizeMACAddress(v.MACAddress),
			ConfigType:          string(v.ConfigType),
			MTU:                 v.MTU,
			ProviderId:          string(v.ProviderId),
			ProviderNetworkId:   string(v.ProviderNetworkId),
			ProviderSubnetId:    string(v.ProviderSubnetId),
			ProviderSpaceId:     string(v.ProviderSpaceId),
			ProviderVLANId:      string(v.ProviderVLANId),
			ProviderAddressId:   string(v.ProviderAddressId),
			VLANTag:             v.VLANTag,
			InterfaceName:       v.InterfaceName,
			ParentInterfaceName: v.ParentInterfaceName,
			InterfaceType:       string(v.InterfaceType),
			Disabled:            v.Disabled,
			NoAutoStart:         v.NoAutoStart,
			Addresses:           FromProviderAddresses(v.Addresses...),
			ShadowAddresses:     FromProviderAddresses(v.ShadowAddresses...),
			DNSServers:          dnsServers,
			DNSSearchDomains:    v.DNSSearchDomains,
			GatewayAddress:      v.GatewayAddress.Value,
			Routes:              routes,
			IsDefaultGateway:    v.IsDefaultGateway,
			VirtualPortType:     string(v.VirtualPortType),
			NetworkOrigin:       NetworkOrigin(v.Origin),

			// TODO (manadart 2021-03-24): Retained for compatibility.
			// Delete CIDR for Juju 3/4.
			CIDR: v.PrimaryAddress().CIDR,
		}
	}
	return result
}

// InterfaceInfoFromNetworkConfig converts a slice of NetworkConfig into the
// equivalent network.InterfaceInfo slice.
func InterfaceInfoFromNetworkConfig(configs []NetworkConfig) network.InterfaceInfos {
	result := make(network.InterfaceInfos, len(configs))
	for i, v := range configs {
		var routes []network.Route
		if len(v.Routes) != 0 {
			routes = make([]network.Route, len(v.Routes))
			for j, route := range v.Routes {
				routes[j] = network.Route{
					DestinationCIDR: route.DestinationCIDR,
					GatewayIP:       route.GatewayIP,
					Metric:          route.Metric,
				}
			}
		}

		configType := network.AddressConfigType(v.ConfigType)

		result[i] = network.InterfaceInfo{
			DeviceIndex:         v.DeviceIndex,
			MACAddress:          network.NormalizeMACAddress(v.MACAddress),
			MTU:                 v.MTU,
			ProviderId:          network.Id(v.ProviderId),
			ProviderNetworkId:   network.Id(v.ProviderNetworkId),
			ProviderSubnetId:    network.Id(v.ProviderSubnetId),
			ProviderSpaceId:     network.Id(v.ProviderSpaceId),
			ProviderVLANId:      network.Id(v.ProviderVLANId),
			ProviderAddressId:   network.Id(v.ProviderAddressId),
			VLANTag:             v.VLANTag,
			InterfaceName:       v.InterfaceName,
			ParentInterfaceName: v.ParentInterfaceName,
			InterfaceType:       network.LinkLayerDeviceType(v.InterfaceType),
			Disabled:            v.Disabled,
			NoAutoStart:         v.NoAutoStart,
			ConfigType:          configType,
			Addresses:           ToProviderAddresses(v.Addresses...),
			ShadowAddresses:     ToProviderAddresses(v.ShadowAddresses...),
			DNSServers:          network.NewMachineAddresses(v.DNSServers).AsProviderAddresses(),
			DNSSearchDomains:    v.DNSSearchDomains,
			GatewayAddress:      network.NewMachineAddress(v.GatewayAddress).AsProviderAddress(),
			Routes:              routes,
			IsDefaultGateway:    v.IsDefaultGateway,
			VirtualPortType:     network.VirtualPortType(v.VirtualPortType),
			Origin:              network.Origin(v.NetworkOrigin),
		}

		// Compatibility accommodations follow.
		// TODO (manadart 2021-03-05): Juju 3/4 should require that only the
		// address collections are used, and the following fields removed from
		// the top-level interface:
		// - CIDR

		// 1) For clients that populate Addresses, but still set
		//    address-specific fields on the device.
		//    Note that the assumption must hold (as it does at the time of
		//    writing) that the collections are only populated with a single
		//    member, with repeated devices for each address.
		if len(result[i].Addresses) > 0 {
			if result[i].Addresses[0].CIDR == "" {
				result[i].Addresses[0].CIDR = v.CIDR
			}
			if result[i].Addresses[0].ConfigType == "" {
				result[i].Addresses[0].ConfigType = configType
			}
		}
	}

	return result
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

	// Endpoint can be left empty to indicate that this port range applies
	// to all application endpoints.
	Endpoint string `json:"endpoint"`
}

// Address represents the location of a machine, including metadata
// about what kind of location the address describes.
// See also the address types in core/network that this type can be
// transformed to/from.
// TODO (manadart 2021-03-05): CIDR is here to correct the old cardinality
// mismatch of having it on the parent NetworkConfig.
// Once we are liberated from backwards compatibility concerns (Juju 3/4),
// we should consider just ensuring that the Value field is in CIDR form.
// This way we can push any required parsing to a single location server-side
// instead of doing it for every implementation of environ.NetworkInterfaces
// plus on-machine detection.
// There are cases when we convert it *back* to the ip/mask form anyway.
type Address struct {
	Value           string `json:"value"`
	CIDR            string `json:"cidr,omitempty"`
	Type            string `json:"type"`
	Scope           string `json:"scope"`
	SpaceName       string `json:"space-name,omitempty"`
	ProviderSpaceID string `json:"space-id,omitempty"`
	ConfigType      string `json:"config-type,omitempty"`
	IsSecondary     bool   `json:"is-secondary,omitempty"`
}

// MachineAddress transforms the Address to a MachineAddress,
// effectively ignoring the space fields.
func (addr Address) MachineAddress() network.MachineAddress {
	return network.MachineAddress{
		Value:       addr.Value,
		CIDR:        addr.CIDR,
		Type:        network.AddressType(addr.Type),
		Scope:       network.Scope(addr.Scope),
		ConfigType:  network.AddressConfigType(addr.ConfigType),
		IsSecondary: addr.IsSecondary,
	}
}

// ProviderAddress transforms the Address to a ProviderAddress.
func (addr Address) ProviderAddress() network.ProviderAddress {
	return network.ProviderAddress{
		MachineAddress:  addr.MachineAddress(),
		SpaceName:       network.SpaceName(addr.SpaceName),
		ProviderSpaceID: network.Id(addr.ProviderSpaceID),
	}
}

// ToProviderAddresses transforms multiple Addresses into a
// ProviderAddresses collection.
func ToProviderAddresses(addrs ...Address) network.ProviderAddresses {
	if len(addrs) == 0 {
		return nil
	}

	pAddrs := make([]network.ProviderAddress, len(addrs))
	for i, addr := range addrs {
		pAddrs[i] = addr.ProviderAddress()
	}
	return pAddrs
}

// FromProviderAddresses transforms multiple ProviderAddresses
// into a slice of Address.
func FromProviderAddresses(pAddrs ...network.ProviderAddress) []Address {
	if len(pAddrs) == 0 {
		return nil
	}

	addrs := make([]Address, len(pAddrs))
	for i, pAddr := range pAddrs {
		addrs[i] = FromProviderAddress(pAddr)
	}
	return addrs
}

// FromProviderAddress returns an Address for the input ProviderAddress.
func FromProviderAddress(addr network.ProviderAddress) Address {
	return Address{
		Value:           addr.Value,
		CIDR:            addr.CIDR,
		Type:            string(addr.Type),
		Scope:           string(addr.Scope),
		SpaceName:       string(addr.SpaceName),
		ProviderSpaceID: string(addr.ProviderSpaceID),
		ConfigType:      string(addr.ConfigType),
		IsSecondary:     addr.IsSecondary,
	}
}

// FromMachineAddresses transforms multiple MachineAddresses
// into a slice of Address.
func FromMachineAddresses(mAddrs ...network.MachineAddress) []Address {
	addrs := make([]Address, len(mAddrs))
	for i, mAddr := range mAddrs {
		addrs[i] = FromMachineAddress(mAddr)
	}
	return addrs
}

// FromMachineAddress returns an Address for the input MachineAddress.
func FromMachineAddress(addr network.MachineAddress) Address {
	return Address{
		Value:       addr.Value,
		CIDR:        addr.CIDR,
		Type:        string(addr.Type),
		Scope:       string(addr.Scope),
		ConfigType:  string(addr.ConfigType),
		IsSecondary: addr.IsSecondary,
	}
}

// HostPort associates an address with a port. It's used in
// the API requests/responses. See also network.HostPort, from/to
// which this is transformed.
type HostPort struct {
	Address
	Port int `json:"port"`
}

// MachineHostPort transforms the HostPort to a MachineHostPort.
func (hp HostPort) MachineHostPort() network.MachineHostPort {
	return network.MachineHostPort{MachineAddress: hp.Address.MachineAddress(), NetPort: network.NetPort(hp.Port)}
}

// ToMachineHostsPorts transforms slices of HostPort grouped by server into
// a slice of MachineHostPorts collections.
func ToMachineHostsPorts(hpm [][]HostPort) []network.MachineHostPorts {
	mHpm := make([]network.MachineHostPorts, len(hpm))
	for i, hps := range hpm {
		mHpm[i] = ToMachineHostPorts(hps)
	}
	return mHpm
}

// ToMachineHostPorts transforms multiple Addresses into a
// MachineHostPort collection.
func ToMachineHostPorts(hps []HostPort) network.MachineHostPorts {
	mHps := make(network.MachineHostPorts, len(hps))
	for i, hp := range hps {
		mHps[i] = hp.MachineHostPort()
	}
	return mHps
}

// ProviderHostPort transforms the HostPort to a ProviderHostPort.
func (hp HostPort) ProviderHostPort() network.ProviderHostPort {
	return network.ProviderHostPort{ProviderAddress: hp.Address.ProviderAddress(), NetPort: network.NetPort(hp.Port)}
}

// ToProviderHostsPorts transforms slices of HostPort grouped by server into
// a slice of ProviderHostPort collections.
func ToProviderHostsPorts(hpm [][]HostPort) []network.ProviderHostPorts {
	pHpm := make([]network.ProviderHostPorts, len(hpm))
	for i, hps := range hpm {
		pHpm[i] = ToProviderHostPorts(hps)
	}
	return pHpm
}

// ToProviderHostPorts transforms multiple Addresses into a
// ProviderHostPorts collection.
func ToProviderHostPorts(hps []HostPort) network.ProviderHostPorts {
	pHps := make(network.ProviderHostPorts, len(hps))
	for i, hp := range hps {
		pHps[i] = hp.ProviderHostPort()
	}
	return pHps
}

// FromProviderHostsPorts is a helper to create a parameter
// out of the network type, here for a nested slice of HostPort.
func FromProviderHostsPorts(nhpm []network.ProviderHostPorts) [][]HostPort {
	hpm := make([][]HostPort, len(nhpm))
	for i, nhps := range nhpm {
		hpm[i] = FromProviderHostPorts(nhps)
	}
	return hpm
}

// FromProviderHostPorts is a helper to create a parameter
// out of the network type, here for a slice of HostPort.
func FromProviderHostPorts(nhps network.ProviderHostPorts) []HostPort {
	hps := make([]HostPort, len(nhps))
	for i, nhp := range nhps {
		hps[i] = FromProviderHostPort(nhp)
	}
	return hps
}

// FromProviderHostPort is a convenience helper to create a parameter
// out of the network type, here for ProviderHostPort.
func FromProviderHostPort(nhp network.ProviderHostPort) HostPort {
	return HostPort{FromProviderAddress(nhp.ProviderAddress), nhp.Port()}
}

// FromHostsPorts is a helper to create a parameter
// out of the network type, here for a nested slice of HostPort.
func FromHostsPorts(nhpm []network.HostPorts) [][]HostPort {
	hpm := make([][]HostPort, len(nhpm))
	for i, nhps := range nhpm {
		hpm[i] = FromHostPorts(nhps)
	}
	return hpm
}

// FromHostPorts is a helper to create a parameter
// out of the network type, here for a slice of HostPort.
func FromHostPorts(nhps network.HostPorts) []HostPort {
	hps := make([]HostPort, len(nhps))
	for i, nhp := range nhps {
		hps[i] = FromHostPort(nhp)
	}
	return hps
}

// FromHostPort is a convenience helper to create a parameter
// out of the network type, here for HostPort.
func FromHostPort(nhp network.HostPort) HostPort {
	return HostPort{
		Address: Address{
			Value: nhp.Host(),
			Type:  string(nhp.AddressType()),
			Scope: string(nhp.AddressScope()),
		},
		Port: nhp.Port(),
	}
}

// SetProviderNetworkConfig holds a slice of machine network configs sourced
// from a provider.
type SetProviderNetworkConfig struct {
	Args []ProviderNetworkConfig `json:"args"`
}

// ProviderNetworkConfig holds a machine tag and a list of network interface
// info obtained by querying the provider.
type ProviderNetworkConfig struct {
	Tag     string          `json:"tag"`
	Configs []NetworkConfig `json:"config"`
}

// SetProviderNetworkConfigResults holds a list of SetProviderNetwork config
// results.
type SetProviderNetworkConfigResults struct {
	Results []SetProviderNetworkConfigResult `json:"results"`
}

// SetProviderNetworkConfigResult holds a list of provider addresses or an
// error.
type SetProviderNetworkConfigResult struct {
	Error     *Error    `json:"error,omitempty"`
	Addresses []Address `json:"addresses"`

	// Modified will be set to true if the provider address list has been
	// updated.
	Modified bool `json:"modified"`
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

// BackFillMachineOrigin sets all empty NetworkOrigin entries to indicate that
// they are sourced from the local machine.
// TODO (manadart 2020-05-12): This is used by superseded methods on the
// Machiner and NetworkConfig APIs, which along with this should considered for
// removing for Juju 3.0.
func (c *SetMachineNetworkConfig) BackFillMachineOrigin() {
	for i := range c.Config {
		if c.Config[i].NetworkOrigin != "" {
			continue
		}
		c.Config[i].NetworkOrigin = NetworkOrigin(network.OriginMachine)
	}
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

// ApplicationOpenedPorts describes the set of port ranges that have been
// opened by an application for an endpoint.
type ApplicationOpenedPorts struct {
	Endpoint   string      `json:"endpoint"`
	PortRanges []PortRange `json:"port-ranges"`
}

// ApplicationOpenedPortsResult holds a single result of the
// CAASFirewallerEmbedded.GetOpenedPorts() API calls.
type ApplicationOpenedPortsResult struct {
	Error                 *Error                   `json:"error,omitempty"`
	ApplicationPortRanges []ApplicationOpenedPorts `json:"application-port-ranges"`
}

// ApplicationOpenedPortsResults holds all the results of the
// CAASFirewallerEmbedded.GetOpenedPorts() API calls.
type ApplicationOpenedPortsResults struct {
	Results []ApplicationOpenedPortsResult `json:"results"`
}

// OpenPortRangesByEndpointResults holds the results of a request to the
// uniter's OpenedMachinePortRangesByEndpoint and OpenedPortRangesByEndpoint API.
type OpenPortRangesByEndpointResults struct {
	Results []OpenPortRangesByEndpointResult `json:"results"`
}

// OpenPortRangesByEndpointResult holds a single result of a request to
// the uniter's OpenedMachinePortRangesByEndpoint and OpenedPortRangesByEndpoint API.
type OpenPortRangesByEndpointResult struct {
	Error *Error `json:"error,omitempty"`

	// The set of opened port ranges grouped by unit tag.
	UnitPortRanges map[string][]OpenUnitPortRangesByEndpoint `json:"unit-port-ranges"`
}

// OpenUnitPortRangesByEndpoint describes the set of port ranges that have been
// opened by a unit on the machine it is deployed to for an endpoint.
type OpenUnitPortRangesByEndpoint struct {
	Endpoint   string      `json:"endpoint"`
	PortRanges []PortRange `json:"port-ranges"`
}

// OpenMachinePortRangesResults holds the results of a request to the
// firewaller's OpenedMachinePortRanges API.
type OpenMachinePortRangesResults struct {
	Results []OpenMachinePortRangesResult `json:"results"`
}

// OpenMachinePortRangesResult holds a single result of a request to
// the firewaller's OpenedMachinePortRanges API.
type OpenMachinePortRangesResult struct {
	Error *Error `json:"error,omitempty"`

	// The set of opened port ranges grouped by unit tag.
	UnitPortRanges map[string][]OpenUnitPortRanges `json:"unit-port-ranges"`
}

// OpenUnitPortRanges describes the set of port ranges that have been
// opened by a unit on the machine it is deployed to for an endpoint.
type OpenUnitPortRanges struct {
	Endpoint   string      `json:"endpoint"`
	PortRanges []PortRange `json:"port-ranges"`

	// The CIDRs that correspond to the subnets assigned to the space that
	// this endpoint is bound to.
	SubnetCIDRs []string `json:"subnet-cidrs"`
}

type IngressRulesResult struct {
	Rules []IngressRule `json:"rules"`
	Error *Error        `json:"error,omitempty"`
}

type IngressRule struct {
	PortRange   PortRange `json:"port-range"`
	SourceCIDRs []string  `json:"source-cidrs"`
}

// APIHostPortsResult holds the result of an APIHostPorts
// call. Each element in the top level slice holds
// the addresses for one API server.
type APIHostPortsResult struct {
	Servers [][]HostPort `json:"servers"`
}

// MachineHostsPorts transforms the APIHostPortsResult into a slice of
// MachineHostPorts.
func (r APIHostPortsResult) MachineHostsPorts() []network.MachineHostPorts {
	return ToMachineHostsPorts(r.Servers)
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

// RemoveSpaceParam holds a single space tag and whether it should be forced.
type RemoveSpaceParam struct {
	Space Entity `json:"space"`
	// Force specifies whether the space removal will be forced, even if existing bindings, constraints or configurations are found.
	Force bool `json:"force,omitempty"`
	// DryRun specifies whether this command should only be run to return which constraints, bindings and configs are using given space.
	// Without applying the remove operations.
	DryRun bool `json:"dry-run,omitempty"`
}

// RemoveSpaceParams holds a single space tag and whether it should be forced.
type RemoveSpaceParams struct {
	SpaceParams []RemoveSpaceParam `json:"space-param"`
}

// RemoveSpaceResults contains multiple RemoveSpace results.
type RemoveSpaceResults struct {
	Results []RemoveSpaceResult `json:"results"`
}

// RemoveSpaceResult contains entries if removing a space is not possible.
// Constraints are a slice of entities which has constraints on the space.
// Bindings are a slice of entities which has bindings on that space.
// Error is filled if an error has occurred which is unexpected.
type RemoveSpaceResult struct {
	Constraints        []Entity `json:"constraints,omitempty"`
	Bindings           []Entity `json:"bindings,omitempty"`
	ControllerSettings []string `json:"controller-settings,omitempty"`
	Error              *Error   `json:"error,omitempty"`
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

// AddSubnetParams holds a cidr and space tags, subnet provider ID,
// and a list of zones to associate the subnet to. Either SubnetTag or
// SubnetProviderId must be set, but not both. Zones can be empty if
// they can be discovered
type AddSubnetParams struct {
	CIDR              string   `json:"cidr,omitempty"`
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

// RenameSpaceParams holds params to rename a space.
// A `from` and `to` space tag.
type RenameSpaceParams struct {
	FromSpaceTag string `json:"from-space-tag"`
	ToSpaceTag   string `json:"to-space-tag"`
}

// RenameSpacesParams holds the arguments of the RenameSpaces API call.
type RenameSpacesParams struct {
	Changes []RenameSpaceParams `json:"changes"`
}

// CreateSpacesParams holds the arguments of the AddSpaces API call.
type CreateSpacesParams struct {
	Spaces []CreateSpaceParams `json:"spaces"`
}

// CreateSpaceParams holds the space tag and at least one subnet
// tag required to create a new space.
type CreateSpaceParams struct {
	CIDRs      []string `json:"cidrs"`
	SpaceTag   string   `json:"space-tag"`
	Public     bool     `json:"public"`
	ProviderId string   `json:"provider-id,omitempty"`
}

// MoveSubnetsParam contains the information required to
// move a collection of subnets into a space.
type MoveSubnetsParam struct {
	// SubnetTags identifies the subnets to move.
	SubnetTags []string `json:"subnets"`

	// SpaceTag identifies the space that the subnets will move to.
	SpaceTag string `json:"space-tag"`

	// Force, when true, moves the subnets despite existing constraints that
	// might be violated by such a topology change.
	Force bool `json:"force"`
}

// MoveSubnetsParams contains the arguments of MoveSubnets API call.
type MoveSubnetsParams struct {
	Args []MoveSubnetsParam `json:"args"`
}

// MovedSubnet represents the prior state of a relocated subnet.
type MovedSubnet struct {
	// SubnetTag identifies the subnet that was moved.
	SubnetTag string `json:"subnet"`

	// OldSpaceTag identifies the space that the subnet was in before being
	// successfully moved.
	OldSpaceTag string `json:"old-space"`

	// CIDR identifies the moved CIDR in the subnet move.
	CIDR string `json:"cidr"`
}

// MoveSubnetsResult contains the result of moving
// a collection of subnets into a new space.
type MoveSubnetsResult struct {
	// MovedSubnets contains the prior state of relocated subnets.
	MovedSubnets []MovedSubnet `json:"moved-subnets,omitempty"`

	// NewSpaceTag identifies the space that the the subnets were moved to.
	// It is intended to facilitate from/to confirmation messages without
	// clients needing to match up parameters with results.
	NewSpaceTag string `json:"new-space"`

	// Error will be non-nil if the subnets could not be moved.
	Error *Error `json:"error,omitempty"`
}

// MoveSubnetsResults contains the results of a call to MoveSubnets.
type MoveSubnetsResults struct {
	Results []MoveSubnetsResult `json:"results"`
}

// ShowSpaceResult holds the list of all available spaces.
type ShowSpaceResult struct {
	// Information about a given space.
	Space Space `json:"space"`
	// Application names which are bound to a given space.
	Applications []string `json:"applications"`
	// MachineCount is the number of machines connected to a given space.
	MachineCount int    `json:"machine-count"`
	Error        *Error `json:"error,omitempty"`
}

// ShowSpaceResults holds the list of all available spaces.
type ShowSpaceResults struct {
	Results []ShowSpaceResult `json:"results"`
}

// ListSpacesResults holds the list of all available spaces.
type ListSpacesResults struct {
	Results []Space `json:"results"`
}

// Space holds the information about a single space and its associated subnets.
type Space struct {
	Id      string   `json:"id"`
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
	SnapStoreProxyURL        string      `json:"snap-store-proxy-url,omitempty"`
	AptMirror                string      `json:"apt-mirror,omitempty"`
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
// The serialization is different between json and yaml because of accidental
// differences in the past, but should be preserved for compatibility
type InterfaceAddress struct {
	Hostname string `json:"hostname" yaml:"hostname"`
	Address  string `json:"value" yaml:"address"`
	CIDR     string `json:"cidr" yaml:"cidr"`
}

// NetworkInfo describes one interface with IP addresses.
// The serialization is different between json and yaml because of accidental
// differences in the past, but should be preserved for compatibility
type NetworkInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string `json:"mac-address" yaml:"macaddress"`

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string `json:"interface-name" yaml:"interfacename"`

	// Addresses contains a list of addresses configured on the interface.
	Addresses []InterfaceAddress `json:"addresses" yaml:"addresses"`
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

// NetworkInfoParams holds a name of the unit and list of bindings for which we want to get NetworkInfos.
type NetworkInfoParams struct {
	Unit       string `json:"unit"`
	RelationId *int   `json:"relation-id,omitempty"`
	// TODO (manadart 2019-10-28): The name of this member was changed to
	// better indicate what it is, but the encoded name was left as-is to
	// avoid the need for facade schema regeneration.
	// Change it to "endpoints" if bumping the facade version for another
	// purpose.
	Endpoints []string `json:"bindings"`
}

// FanConfigEntry holds configuration for a single fan.
type FanConfigEntry struct {
	Underlay string `json:"underlay"`
	Overlay  string `json:"overlay"`
}

// FanConfigResult holds configuration for all fans in a model.
type FanConfigResult struct {
	Fans []FanConfigEntry `json:"fans"`
}

// CIDRParams contains a slice of subnet CIDRs used for querying subnets.
type CIDRParams struct {
	CIDRS []string `json:"cidrs"`
}

// SubnetsResult contains a collection of subnets or an error.
type SubnetsResult struct {
	Subnets []SubnetV2 `json:"subnets,omitempty"`
	Error   *Error     `json:"error,omitempty"`
}

// SubnetsResults contains a collection of subnets results.
type SubnetsResults struct {
	Results []SubnetsResult `json:"results"`
}

// SpaceInfosParams provides the arguments for a SpaceInfos call.
type SpaceInfosParams struct {
	// A list of space IDs for filtering the returned set of results. If
	// empty, all spaces will be returned.
	FilterBySpaceIDs []string `json:"space-ids,omitempty"`
}

// SpaceInfos represents the result of a SpaceInfos API call.
type SpaceInfos struct {
	Infos []SpaceInfo `json:"space-infos,omitempty"`
}

// SpaceInfo describes a space and its subnets.
type SpaceInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ProviderID string     `json:"provider-id,omitempty"`
	Subnets    []SubnetV3 `json:"subnets,omitempty"`
}

// FromNetworkSpaceInfos converts a network.SpaceInfos into a serializable
// payload that can be sent over the wire.
func FromNetworkSpaceInfos(allInfos network.SpaceInfos) SpaceInfos {
	res := SpaceInfos{
		Infos: make([]SpaceInfo, len(allInfos)),
	}

	for i, si := range allInfos {
		mappedSubnets := make([]SubnetV3, len(si.Subnets))
		for j, subnetInfo := range si.Subnets {

			mappedSubnets[j] = SubnetV3{
				SpaceID: subnetInfo.SpaceID,

				SubnetV2: SubnetV2{
					ID: string(subnetInfo.ID),
					Subnet: Subnet{
						CIDR:              subnetInfo.CIDR,
						ProviderId:        string(subnetInfo.ProviderId),
						ProviderNetworkId: string(subnetInfo.ProviderNetworkId),
						ProviderSpaceId:   string(subnetInfo.ProviderSpaceId),
						VLANTag:           subnetInfo.VLANTag,
						Zones:             subnetInfo.AvailabilityZones,
						// NOTE(achilleasa): the SpaceTag is not populated
						// as we can grab the space name from the parent
						// SpaceInfo when unmarshaling.
					},
				},
			}
		}

		res.Infos[i] = SpaceInfo{
			ID:         si.ID,
			Name:       string(si.Name),
			ProviderID: string(si.ProviderId),
			Subnets:    mappedSubnets,
		}
	}

	return res
}

// ToNetworkSpaceInfos converts a serializable SpaceInfos payload into a
// network.SpaceInfos instance.
func ToNetworkSpaceInfos(allInfos SpaceInfos) network.SpaceInfos {
	res := make(network.SpaceInfos, len(allInfos.Infos))

	for i, si := range allInfos.Infos {
		mappedSubnets := make(network.SubnetInfos, len(si.Subnets))
		for j, subnetInfo := range si.Subnets {
			mappedSubnets[j] = network.SubnetInfo{
				ID:                network.Id(subnetInfo.ID),
				CIDR:              subnetInfo.CIDR,
				ProviderId:        network.Id(subnetInfo.ProviderId),
				ProviderSpaceId:   network.Id(subnetInfo.ProviderSpaceId),
				ProviderNetworkId: network.Id(subnetInfo.ProviderNetworkId),
				VLANTag:           subnetInfo.VLANTag,
				AvailabilityZones: subnetInfo.Zones,
				SpaceID:           subnetInfo.SpaceID,
				SpaceName:         si.Name,
			}
		}

		res[i] = network.SpaceInfo{
			ID:         si.ID,
			Name:       network.SpaceName(si.Name),
			ProviderId: network.Id(si.ProviderID),
			Subnets:    mappedSubnets,
		}
	}

	return res
}
