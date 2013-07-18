// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"net"
)

// AddressType represents the possible ways of specifying a machine location by
// either a hostname resolvable by dns lookup, or ipv4 or ipv6 address.
type AddressType string

const (
	HostName    AddressType = "hostname"
	Ipv4Address AddressType = "ipv4"
	Ipv6Address AddressType = "ipv6"
)

// NetworkScope denotes the context a location may apply to. If a name or
// address can be reached from the wider internet, it is considered public. A
// private network address is either specific to the cloud or cloud subnet a
// machine belongs to, or to the machine itself for containers.
type NetworkScope string

const (
	NetworkUnknown      NetworkScope = ""
	NetworkPublic       NetworkScope = "public"
	NetworkCloudLocal   NetworkScope = "local-cloud"
	NetworkMachineLocal NetworkScope = "local-machine"
)

// Address represents the location of a machine, including metadata about what
// kind of location the address describes.
type Address struct {
	Value       string
	Type        AddressType
	NetworkName string
	NetworkScope
}

func deriveAddressType(value string) AddressType {
	ip := net.ParseIP(value)
	if ip != nil {
		if ip.To4() != nil {
			return Ipv4Address
		}
		if ip.To16() != nil {
			return Ipv6Address
		}
		panic("Unknown form of IP address")
	}
	// TODO(gz): Check value is a valid hostname
	return HostName
}

func NewAddress(value string) Address {
	addresstype := deriveAddressType(value)
	return Address{value, addresstype, "", NetworkUnknown}
}
