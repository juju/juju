// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"

	"github.com/juju/errors"
)

// InterfaceConfigType defines valid network interface configuration
// types. See interfaces(5) for details
type InterfaceConfigType string

const (
	ConfigUnknown  InterfaceConfigType = ""
	ConfigDHCP     InterfaceConfigType = "dhcp"
	ConfigStatic   InterfaceConfigType = "static"
	ConfigManual   InterfaceConfigType = "manual"
	ConfigLoopback InterfaceConfigType = "loopback"
)

// InterfaceType defines valid network interface types.
type InterfaceType string

const (
	UnknownInterface    InterfaceType = ""
	LoopbackInterface   InterfaceType = "loopback"
	EthernetInterface   InterfaceType = "ethernet"
	VLAN_8021QInterface InterfaceType = "802.1q"
	BondInterface       InterfaceType = "bond"
	BridgeInterface     InterfaceType = "bridge"
)

// Route defines a single route to a subnet via a defined gateway.
type Route struct {
	// DestinationCIDR is the subnet that we want a controlled route to.
	DestinationCIDR string
	// GatewayIP is the IP (v4 or v6) that should be used for traffic that is
	// bound for DestinationCIDR
	GatewayIP string
	// Metric is the weight to apply to this route.
	Metric int
}

// Validate that this Route is properly formed.
func (r Route) Validate() error {
	// Make sure the CIDR is actually a CIDR not just an IP or hostname
	destinationIP, _, err := net.ParseCIDR(r.DestinationCIDR)
	if err != nil {
		return errors.Annotate(err, "DestinationCIDR not valid")
	}
	// Make sure the Gateway is just an IP, not a CIDR, etc.
	gatewayIP := net.ParseIP(r.GatewayIP)
	if gatewayIP == nil {
		return errors.Errorf("GatewayIP is not a valid IP address: %q", r.GatewayIP)
	}
	if r.Metric < 0 {
		return errors.Errorf("Metric is negative: %d", r.Metric)
	}
	// Make sure that either both are IPv4 or both are IPv6, not mixed.
	destIP4 := destinationIP.To4()
	gatewayIP4 := gatewayIP.To4()
	if destIP4 != nil {
		if gatewayIP4 == nil {
			return errors.Errorf("DestinationCIDR is IPv4 (%s) but GatewayIP is IPv6 (%s)", r.DestinationCIDR, r.GatewayIP)
		}
	} else {
		if gatewayIP4 != nil {
			return errors.Errorf("DestinationCIDR is IPv6 (%s) but GatewayIP is IPv4 (%s)", r.DestinationCIDR, r.GatewayIP)
		}
	}
	return nil
}

// InterfaceInfo describes a single network interface available on an
// instance.
type InterfaceInfo struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// ProviderSubnetId is the provider-specific id for the associated
	// subnet.
	ProviderSubnetId Id

	// ProviderNetworkId is the provider-specific id for the
	// associated network.
	ProviderNetworkId Id

	// ProviderSpaceId is the provider-specific id for the associated space,
	// if known and supported.
	ProviderSpaceId Id

	// ProviderVLANId is the provider-specific id of the VLAN for this
	// interface.
	ProviderVLANId Id

	// ProviderAddressId is the provider-specific id of the assigned address.
	ProviderAddressId Id

	// AvailabilityZones describes the availability zones the associated
	// subnet is in.
	AvailabilityZones []string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// ParentInterfaceName is the name of the parent interface to use,
	// if known.
	ParentInterfaceName string

	// InterfaceType is the type of the interface.
	InterfaceType InterfaceType

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it.
	Disabled bool

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot.
	// By default and for backwards-compatibility, interfaces are
	// configured to auto-start.
	NoAutoStart bool

	// ConfigType determines whether the interface should be
	// configured via DHCP, statically, manually, etc. See
	// interfaces(5) for more information.
	ConfigType InterfaceConfigType

	// Addresses contains an optional list of static IP address to
	// configure for this network interface. The subnet mask to set will be
	// inferred from the CIDR value of the first entry which is always
	// assumed to be the primary IP address for the interface.
	Addresses ProviderAddresses

	// ShadowAddresses contains an optional list of additional IP addresses
	// that the underlying network provider associates with this network
	// interface instance. These IP addresses are not typically visible
	// to the machine that the interface is connected to.
	ShadowAddresses ProviderAddresses

	// DNSServers contains an optional list of IP addresses and/or
	// host names to configure as DNS servers for this network interface.
	DNSServers ProviderAddresses

	// MTU is the Maximum Transmission Unit controlling the maximum size of the
	// protocol packets that the interface can pass through. It is only used
	// when > 0.
	MTU int

	// DNSSearchDomains contains the default DNS domain to use for non-FQDN
	// lookups.
	DNSSearchDomains []string

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually is (one of) the host address(es).
	GatewayAddress ProviderAddress

	// Routes defines a list of routes that should be added when this interface
	// is brought up, and removed when this interface is stopped.
	Routes []Route

	// IsDefaultGateway is set if this device is a default gw on a machine.
	IsDefaultGateway bool

	// Origin represents the authoritative source of the InterfaceInfo.
	// It is expected that either the provider gave us this info or the
	// machine gave us this info.
	// Giving us this information allows us to reason about when a InterfaceInfo
	// is in use.
	Origin Origin
}

// ActualInterfaceName returns raw interface name for raw interface (e.g. "eth0") and
// virtual interface name for virtual interface (e.g. "eth0.42")
func (i *InterfaceInfo) ActualInterfaceName() string {
	if i.VLANTag > 0 {
		return fmt.Sprintf("%s.%d", i.InterfaceName, i.VLANTag)
	}
	return i.InterfaceName
}

// IsVirtual returns true when the interface is a virtual device, as
// opposed to a physical device (e.g. a VLAN or a network alias)
func (i *InterfaceInfo) IsVirtual() bool {
	return i.VLANTag > 0
}

// IsVLAN returns true when the interface is a VLAN interface.
func (i *InterfaceInfo) IsVLAN() bool {
	return i.VLANTag > 0
}

// CIDRAddress returns Address.Value combined with subnet mask.
// TODO (manadart 2020-07-02): Usage of this method should be phased out
// in favour of calling ValueForCIDR on each member of the addresses slice.
func (i *InterfaceInfo) CIDRAddress() (string, error) {
	primaryAddr := i.PrimaryAddress()

	if i.CIDR == "" || primaryAddr.Value == "" {
		return "", errors.NotFoundf("address and CIDR pair (%q, %q)", primaryAddr.Value, i.CIDR)
	}

	withMask, err := primaryAddr.ValueForCIDR(i.CIDR)
	return withMask, errors.Trace(err)
}

// PrimaryAddress returns the primary address for the interface.
func (i *InterfaceInfo) PrimaryAddress() ProviderAddress {
	if len(i.Addresses) == 0 {
		return ProviderAddress{}
	}

	// We assume that the primary IP is always listed first. The majority
	// of providers only define a single IP so this will still work as
	// expected. Notably, ec2 does allow multiple private IP addresses to
	// be assigned to an interface but the provider ensures that the one
	// flagged as primary is present at index 0.
	return i.Addresses[0]
}

// InterfaceInfos is a slice of InterfaceInfo
// for a single host/machine/container.
type InterfaceInfos []InterfaceInfo

// IterHierarchy runs the input function for every interface by processing each
// device hierarchy, ensuring that no child device is processed before its
// parent.
func (s InterfaceInfos) IterHierarchy(f func(InterfaceInfo) error) error {
	return s.iterChildHierarchy("", f)
}

func (s InterfaceInfos) iterChildHierarchy(parentName string, f func(InterfaceInfo) error) error {
	children := s.Children(parentName)
	for _, child := range children {
		if err := f(child); err != nil {
			return err
		}
		if err := s.iterChildHierarchy(child.InterfaceName, f); err != nil {
			return err
		}
	}
	return nil
}

// Children returns interfaces that are direct children
// of the interface with the input name.
func (s InterfaceInfos) Children(parentName string) InterfaceInfos {
	var children InterfaceInfos
	for _, dev := range s {
		if dev.ParentInterfaceName == parentName {
			children = append(children, dev)
		}
	}
	return children
}

// GetByHardwareAddress returns a reference to the interface with the input
// hardware address (such as a MAC address) if it exists in the collection,
// otherwise nil is returned.
func (s InterfaceInfos) GetByHardwareAddress(hwAddr string) *InterfaceInfo {
	for _, dev := range s {
		if dev.MACAddress == hwAddr {
			return &dev
		}
	}
	return nil
}

// ProviderInterfaceInfo holds enough information to identify an
// interface or link layer device to a provider so that it can be
// queried or manipulated. Its initial purpose is to pass to
// provider.ReleaseContainerAddresses.
type ProviderInterfaceInfo struct {
	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string
}
