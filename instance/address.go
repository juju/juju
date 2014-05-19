// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"bytes"
	"net"
	"strconv"
)

// Private network ranges for IPv4.
// See: http://tools.ietf.org/html/rfc1918
var (
	classAPrivate = mustParseCIDR("10.0.0.0/8")
	classBPrivate = mustParseCIDR("172.16.0.0/12")
	classCPrivate = mustParseCIDR("192.168.0.0/16")
)

func mustParseCIDR(s string) *net.IPNet {
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return net
}

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

// HostPort associates an address with a port.
type HostPort struct {
	Address
	Port int
}

// AddressesWithPort returns the given addresses all
// associated with the given port.
func AddressesWithPort(addrs []Address, port int) []HostPort {
	hps := make([]HostPort, len(addrs))
	for i, addr := range addrs {
		hps[i] = HostPort{
			Address: addr,
			Port:    port,
		}
	}
	return hps
}

// NetAddr returns the host-port as an address
// suitable for calling net.Dial.
func (hp HostPort) NetAddr() string {
	return net.JoinHostPort(hp.Value, strconv.Itoa(hp.Port))
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

// NewAddresses is a convenience function to create addresses from a string slice
func NewAddresses(inAddresses ...string) (outAddresses []Address) {
	for _, address := range inAddresses {
		outAddresses = append(outAddresses, NewAddress(address, NetworkUnknown))
	}
	return outAddresses
}

func DeriveAddressType(value string) AddressType {
	ip := net.ParseIP(value)
	switch {
	case ip == nil:
		// TODO(gz): Check value is a valid hostname
		return HostName
	case ip.To4() != nil:
		return Ipv4Address
	case ip.To16() != nil:
		return Ipv6Address
	default:
		panic("Unknown form of IP address")
	}
}

func isIPv4PrivateNetworkAddress(ip net.IP) bool {
	return classAPrivate.Contains(ip) ||
		classBPrivate.Contains(ip) ||
		classCPrivate.Contains(ip)
}

// deriveNetworkScope attempts to derive the network scope from an address's
// type and value, returning the original network scope if no deduction can
// be made.
func deriveNetworkScope(addr Address) NetworkScope {
	if addr.Type == HostName {
		return addr.NetworkScope
	}
	ip := net.ParseIP(addr.Value)
	if ip == nil {
		return addr.NetworkScope
	}
	if ip.IsLoopback() {
		return NetworkMachineLocal
	}
	switch addr.Type {
	case Ipv4Address:
		if isIPv4PrivateNetworkAddress(ip) {
			return NetworkCloudLocal
		}
		// If it's not loopback, and it's not a private
		// network address, then it's publicly routable.
		return NetworkPublic
	case Ipv6Address:
		// TODO(axw) check for IPv6 unique local address, if/when we care.
	}
	return addr.NetworkScope
}

// NewAddress creates a new Address, deriving its type from the value.
//
// If the specified scope is NetworkUnknown, then NewAddress will
// attempt derive the scope based on reserved IP address ranges.
func NewAddress(value string, scope NetworkScope) Address {
	addr := Address{
		Value:        value,
		Type:         DeriveAddressType(value),
		NetworkScope: scope,
	}
	if scope == NetworkUnknown {
		addr.NetworkScope = deriveNetworkScope(addr)
	}
	return addr
}

// SelectPublicAddress picks one address from a slice that would
// be appropriate to display as a publicly accessible endpoint.
// If there are no suitable addresses, the empty string is returned.
func SelectPublicAddress(addresses []Address) string {
	index := bestAddressIndex(len(addresses), func(i int) Address {
		return addresses[i]
	}, publicMatch)
	if index < 0 {
		return ""
	}
	return addresses[index].Value
}

func SelectPublicHostPort(hps []HostPort) string {
	index := bestAddressIndex(len(hps), func(i int) Address {
		return hps[i].Address
	}, publicMatch)
	if index < 0 {
		return ""
	}
	return hps[index].NetAddr()
}

// SelectInternalAddress picks one address from a slice that can be
// used as an endpoint for juju internal communication.
// If there are no suitable addresses, the empty string is returned.
func SelectInternalAddress(addresses []Address, machineLocal bool) string {
	index := bestAddressIndex(len(addresses), func(i int) Address {
		return addresses[i]
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return ""
	}
	return addresses[index].Value
}

// SelectInternalHostPort picks one HostPort from a slice that can be
// used as an endpoint for juju internal communication and returns it
// in its NetAddr form.
// If there are no suitable addresses, the empty string is returned.
func SelectInternalHostPort(hps []HostPort, machineLocal bool) string {
	index := bestAddressIndex(len(hps), func(i int) Address {
		return hps[i].Address
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return ""
	}
	return hps[index].NetAddr()
}

func publicMatch(addr Address) scopeMatch {
	switch addr.NetworkScope {
	case NetworkPublic:
		return exactScope
	case NetworkCloudLocal, NetworkUnknown:
		return fallbackScope
	}
	return invalidScope
}

func internalAddressMatcher(machineLocal bool) func(Address) scopeMatch {
	if machineLocal {
		return cloudOrMachineLocalMatch
	}
	return cloudLocalMatch
}

func cloudLocalMatch(addr Address) scopeMatch {
	switch addr.NetworkScope {
	case NetworkCloudLocal:
		return exactScope
	case NetworkPublic, NetworkUnknown:
		return fallbackScope
	}
	return invalidScope
}

func cloudOrMachineLocalMatch(addr Address) scopeMatch {
	if addr.NetworkScope == NetworkMachineLocal {
		return exactScope
	}
	return cloudLocalMatch(addr)
}

type scopeMatch int

const (
	invalidScope scopeMatch = iota
	exactScope
	fallbackScope
)

// bestAddressIndex returns the index of the first address
// with an exactly matching scope, or the first address with
// a matching fallback scope if there are no exact matches.
// If there are no suitable addresses, -1 is returned.
func bestAddressIndex(numAddr int, getAddr func(i int) Address, match func(addr Address) scopeMatch) int {
	fallbackAddressIndex := -1
	for i := 0; i < numAddr; i++ {
		addr := getAddr(i)
		if addr.Type != Ipv6Address {
			switch match(addr) {
			case exactScope:
				return i
			case fallbackScope:
				// Use the first fallback address if there are no exact matches.
				if fallbackAddressIndex == -1 {
					fallbackAddressIndex = i
				}
			}
		}
	}
	return fallbackAddressIndex
}
