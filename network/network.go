// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"
	"sort"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.network")

// TODO(dimitern): Remove this once we use spaces as per the model.
const (
	// Id of the default public juju network
	DefaultPublic = "juju-public"

	// Id of the default private juju network
	DefaultPrivate = "juju-private"
)

// Id defines a provider-specific network id.
type Id string

// SubnetInfo describes the bare minimum information for a subnet,
// which the provider knows about but juju might not yet.
type SubnetInfo struct {
	// CIDR of the network, in 123.45.67.89/24 format. Can be empty if
	// unknown.
	CIDR string

	// ProviderId is a provider-specific network id. This the only
	// required field.
	ProviderId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard, and used
	// to define a VLAN network. For more information, see:
	// http://en.wikipedia.org/wiki/IEEE_802.1Q.
	VLANTag int

	// AllocatableIPLow and AllocatableIPHigh describe the allocatable
	// portion of the subnet. The provider will only permit allocation
	// between these limits. If they are empty then none of the subnet is
	// allocatable.
	AllocatableIPLow  net.IP
	AllocatableIPHigh net.IP
}

// InterfaceConfigType defines valid network interface configuration
// types. See interfaces(5) for details
type InterfaceConfigType string

const (
	ConfigUnknown InterfaceConfigType = ""
	ConfigDHCP    InterfaceConfigType = "dhcp"
	ConfigStatic  InterfaceConfigType = "static"
	ConfigManual  InterfaceConfigType = "manual"
	// add others when needed
)

// InterfaceInfo describes a single network interface available on an
// instance. For providers that support networks, this will be
// available at StartInstance() time.
type InterfaceInfo struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// NetworkName is juju-internal name of the network.
	NetworkName string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// ProviderSubnetId is the provider-specific id for the associated
	// subnet.
	ProviderSubnetId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it.
	Disabled bool

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot. By default and for
	// backwards-compatibility, interfaces are configured to
	// auto-start.
	NoAutoStart bool

	// ConfigType determines whether the interface should be
	// configured via DHCP, statically, manually, etc. See
	// interfaces(5) for more information.
	ConfigType InterfaceConfigType

	// Address contains an optional static IP address to configure for
	// this network interface. The subnet mask to set will be inferred
	// from the CIDR value.
	Address Address

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers []Address

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually is (one of) the host address(es).
	GatewayAddress Address

	// ExtraConfig can contain any valid setting and its value allowed
	// inside an "iface" section of a interfaces(5) config file, e.g.
	// "up", "down", "mtu", etc.
	ExtraConfig map[string]string
}

type interfaceInfoSlice []InterfaceInfo

func (s interfaceInfoSlice) Len() int      { return len(s) }
func (s interfaceInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s interfaceInfoSlice) Less(i, j int) bool {
	iface1 := s[i]
	iface2 := s[j]
	return iface1.DeviceIndex < iface2.DeviceIndex
}

// Sort a slice of InterfaceInfo on DeviceIndex in ascending order
func SortInterfaceInfo(interfaces []InterfaceInfo) {
	sort.Sort(interfaceInfoSlice(interfaces))
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

// PreferIPv6Getter will be implemented by both the environment and agent
// config.
type PreferIPv6Getter interface {
	PreferIPv6() bool
}

// InitializeFromConfig needs to be called once after the environment
// or agent configuration is available to configure networking
// settings.
func InitializeFromConfig(config PreferIPv6Getter) {
	globalPreferIPv6 = config.PreferIPv6()
	logger.Infof("setting prefer-ipv6 to %v", globalPreferIPv6)
}
