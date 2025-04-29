// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "github.com/juju/juju/core/network"

// NetAddr represents an IP address and its
// association with a network interface.
type NetAddr struct {
	InterfaceName string
	ProviderID    *network.Id
	AddressValue  string
	CIDR          string
	// ProviderSubnetID is the provider's identity for the subnet that this
	// address is connected to. It is intended to uniquely identify the
	// subnet in the event that the same CIDR is used on multiple provider
	// networks.
	ProviderSubnetID *network.Id
	AddressType      string
	ConfigType       string
	// Origin identifies the authority of this address.
	// I.e. the machine itself, or the provider substrate.
	Origin      network.Origin
	Scope       string
	IsSecondary bool
	IsShadow    bool
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
