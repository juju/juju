// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.network")

// Id of the default public juju network
const DefaultPublic = "juju-public"

// Id of the default private juju network
const DefaultPrivate = "juju-private"

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

// Info describes a single network interface available on an instance.
// For providers that support networks, this will be available at
// StartInstance() time.
type Info struct {
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

	// ProviderId is a provider-specific network id.
	ProviderId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it.
	Disabled bool
}

// ActualInterfaceName returns raw interface name for raw interface (e.g. "eth0") and
// virtual interface name for virtual interface (e.g. "eth0.42")
func (i *Info) ActualInterfaceName() string {
	if i.VLANTag > 0 {
		return fmt.Sprintf("%s.%d", i.InterfaceName, i.VLANTag)
	}
	return i.InterfaceName
}

// IsVirtual returns true when the interface is a virtual device, as
// opposed to a physical device (e.g. a VLAN or a network alias)
func (i *Info) IsVirtual() bool {
	return i.VLANTag > 0
}

// IsVLAN returns true when the interface is a VLAN interface.
func (i *Info) IsVLAN() bool {
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
