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

func DeriveAddressType(value string) AddressType {
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
	addresstype := DeriveAddressType(value)
	return Address{value, addresstype, "", NetworkUnknown}
}

// HostAddresses looks up the IP addresses of the specified
// host, and translates them into instance.Address values.
func HostAddresses(host string) ([]Address, error) {
	ipaddrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	addrs := make([]Address, len(ipaddrs))
	for i, ipaddr := range ipaddrs {
		switch len(ipaddr) {
		case 4:
			addrs[i].Type = Ipv4Address
			addrs[i].Value = ipaddr.String()
		case 16:
			addrs[i].Type = Ipv6Address
			addrs[i].Value = ipaddr.String()
		}
	}
	return addrs, err
}

// SelectPublicAddress picks one address from a slice that would
// be appropriate to display as a publicly accessible endpoint.
// If there are no suitable addresses, the empty string is returned.
func SelectPublicAddress(addresses []Address) string {
	mostpublic := ""
	for _, addr := range addresses {
		if addr.Type != Ipv6Address {
			switch addr.NetworkScope {
			case NetworkPublic:
				return addr.Value
			case NetworkCloudLocal, NetworkUnknown:
				mostpublic = addr.Value
			}
		}
	}
	return mostpublic
}

// SelectInternalAddress picks one address from a slice that can be
// used as an endpoint for juju internal communication.
// If there are no suitable addresses, the empty string is returned.
func SelectInternalAddress(addresses []Address, machineLocal bool) string {
	usableAddress := ""
	for _, addr := range addresses {
		if addr.Type != Ipv6Address {
			switch addr.NetworkScope {
			case NetworkCloudLocal:
				return addr.Value
			case NetworkMachineLocal:
				if machineLocal {
					return addr.Value
				}
			case NetworkPublic, NetworkUnknown:
				if usableAddress == "" {
					usableAddress = addr.Value
				}
			}
		}
	}
	return usableAddress
}
