// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"bytes"
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

// String returns a string representation of the address,
// in the form: scope:address(network name);
// for example:
//
//	public:c2-54-226-162-124.compute-1.amazonaws.com(ec2network)
//
// If the scope is NetworkUnknown, the initial scope: prefix will
// be omitted. If the NetworkName is blank, the (network name) suffix
// will be omitted.
func (a Address) String() string {
	var buf bytes.Buffer
	if a.NetworkScope != NetworkUnknown {
		buf.WriteString(string(a.NetworkScope))
		buf.WriteByte(':')
	}
	buf.WriteString(a.Value)
	if a.NetworkName != "" {
		buf.WriteByte('(')
		buf.WriteString(a.NetworkName)
		buf.WriteByte(')')
	}
	return buf.String()
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

// netLookupIP is a var for testing.
var netLookupIP = net.LookupIP

// HostAddresses looks up the IP addresses of the specified
// host, and translates them into instance.Address values.
//
// The argument passed in is always added ast the final
// address in the resulting slice.
func HostAddresses(host string) (addrs []Address, err error) {
	hostAddr := NewAddress(host)
	if hostAddr.Type != HostName {
		// IPs shouldn't be fed into LookupIP.
		return []Address{hostAddr}, nil
	}
	ipaddrs, err := netLookupIP(host)
	if err != nil {
		return nil, err
	}
	addrs = make([]Address, len(ipaddrs)+1)
	for i, ipaddr := range ipaddrs {
		switch len(ipaddr) {
		case net.IPv4len:
			addrs[i].Type = Ipv4Address
			addrs[i].Value = ipaddr.String()
		case net.IPv6len:
			if ipaddr.To4() != nil {
				// ipaddr is an IPv4 address represented in 16 bytes.
				addrs[i].Type = Ipv4Address
			} else {
				addrs[i].Type = Ipv6Address
			}
			addrs[i].Value = ipaddr.String()
		}
	}
	addrs[len(addrs)-1] = hostAddr
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
