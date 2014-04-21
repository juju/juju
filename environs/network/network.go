// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// Id defines a provider-specific network id.
type Id string

// Info describes a single network interface available on an instance.
// For providers that support networks, this will be available at
// StartInstance() time.
type Info struct {
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

	// InterfaceName is the OS-specific network device name (e.g.
	// "eth0" or "eth1.42" for a VLAN virtual interface).
	InterfaceName string

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device.
	IsVirtual bool
}
