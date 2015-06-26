// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bytes"
	"encoding/binary"
	"net"
	"sort"

	"github.com/juju/errors"
)

// Private network ranges for IPv4 and IPv6.
// See: http://tools.ietf.org/html/rfc1918
// Also: http://tools.ietf.org/html/rfc4193
var (
	classAPrivate   = mustParseCIDR("10.0.0.0/8")
	classBPrivate   = mustParseCIDR("172.16.0.0/12")
	classCPrivate   = mustParseCIDR("192.168.0.0/16")
	ipv6UniqueLocal = mustParseCIDR("fc00::/7")
)

// globalPreferIPv6 determines whether IPv6 addresses will be
// preferred when selecting a public or internal addresses, using the
// Select*() methods below. InitializeFromConfig() needs to be called
// to set this flag globally at the earliest time possible (e.g. at
// bootstrap, agent startup, before any CLI command).
var globalPreferIPv6 bool = false

// ResetGobalPreferIPv6 resets the global variable back to the default,
// and is called only from the isolation test suite to make sure we have
// a clean environment.
func ResetGobalPreferIPv6() {
	globalPreferIPv6 = false
}

func mustParseCIDR(s string) *net.IPNet {
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return net
}

// AddressType represents the possible ways of specifying a machine location by
// either a hostname resolvable by dns lookup, or IPv4 or IPv6 address.
type AddressType string

const (
	HostName    AddressType = "hostname"
	IPv4Address AddressType = "ipv4"
	IPv6Address AddressType = "ipv6"
)

// Scope denotes the context a location may apply to. If a name or
// address can be reached from the wider internet, it is considered
// public. A private network address is either specific to the cloud
// or cloud subnet a machine belongs to, or to the machine itself for
// containers.
type Scope string

const (
	ScopeUnknown      Scope = ""
	ScopePublic       Scope = "public"
	ScopeCloudLocal   Scope = "local-cloud"
	ScopeMachineLocal Scope = "local-machine"
	ScopeLinkLocal    Scope = "link-local"
)

// Address represents the location of a machine, including metadata
// about what kind of location the address describes.
type Address struct {
	Value       string
	Type        AddressType
	NetworkName string
	Scope
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
	if a.Scope != ScopeUnknown {
		buf.WriteString(string(a.Scope))
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

// GoString implements fmt.GoStringer.
func (a Address) GoString() string {
	return a.String()
}

// NewAddress creates a new Address, deriving its type from the value
// and using ScopeUnknown as scope. It's a shortcut to calling
// NewScopedAddress(value, ScopeUnknown).
func NewAddress(value string) Address {
	return NewScopedAddress(value, ScopeUnknown)
}

// NewScopedAddress creates a new Address, deriving its type from the
// value.
//
// If the specified scope is ScopeUnknown, then NewScopedAddress will
// attempt derive the scope based on reserved IP address ranges.
// Because passing ScopeUnknown is fairly common, NewAddress() above
// does exactly that.
func NewScopedAddress(value string, scope Scope) Address {
	addr := Address{
		Value: value,
		Type:  DeriveAddressType(value),
		Scope: scope,
	}
	if scope == ScopeUnknown {
		addr.Scope = deriveScope(addr)
	}
	return addr
}

// NewAddresses is a convenience function to create addresses from a
// string slice.
func NewAddresses(inAddresses ...string) (outAddresses []Address) {
	for _, address := range inAddresses {
		outAddresses = append(outAddresses, NewAddress(address))
	}
	return outAddresses
}

// DeriveAddressType attempts to detect the type of address given.
func DeriveAddressType(value string) AddressType {
	ip := net.ParseIP(value)
	switch {
	case ip == nil:
		// TODO(gz): Check value is a valid hostname
		return HostName
	case ip.To4() != nil:
		return IPv4Address
	case ip.To16() != nil:
		return IPv6Address
	default:
		panic("Unknown form of IP address")
	}
}

func isIPv4PrivateNetworkAddress(addrType AddressType, ip net.IP) bool {
	if addrType != IPv4Address {
		return false
	}
	return classAPrivate.Contains(ip) ||
		classBPrivate.Contains(ip) ||
		classCPrivate.Contains(ip)
}

func isIPv6UniqueLocalAddress(addrType AddressType, ip net.IP) bool {
	if addrType != IPv6Address {
		return false
	}
	return ipv6UniqueLocal.Contains(ip)
}

// deriveScope attempts to derive the network scope from an address's
// type and value, returning the original network scope if no
// deduction can be made.
func deriveScope(addr Address) Scope {
	if addr.Type == HostName {
		return addr.Scope
	}
	ip := net.ParseIP(addr.Value)
	if ip == nil {
		return addr.Scope
	}
	if ip.IsLoopback() {
		return ScopeMachineLocal
	}
	if isIPv4PrivateNetworkAddress(addr.Type, ip) ||
		isIPv6UniqueLocalAddress(addr.Type, ip) {
		return ScopeCloudLocal
	}
	if ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsInterfaceLocalMulticast() {
		return ScopeLinkLocal
	}
	if ip.IsGlobalUnicast() {
		return ScopePublic
	}
	return addr.Scope
}

// SelectPublicAddress picks one address from a slice that would be
// appropriate to display as a publicly accessible endpoint. If there
// are no suitable addresses, the empty string is returned.
func SelectPublicAddress(addresses []Address) string {
	index := bestAddressIndex(len(addresses), globalPreferIPv6, func(i int) Address {
		return addresses[i]
	}, publicMatch)
	if index < 0 {
		return ""
	}
	return addresses[index].Value
}

// SelectPublicHostPort picks one HostPort from a slice that would be
// appropriate to display as a publicly accessible endpoint. If there
// are no suitable candidates, the empty string is returned.
func SelectPublicHostPort(hps []HostPort) string {
	index := bestAddressIndex(len(hps), globalPreferIPv6, func(i int) Address {
		return hps[i].Address
	}, publicMatch)
	if index < 0 {
		return ""
	}
	return hps[index].NetAddr()
}

// SelectInternalAddress picks one address from a slice that can be
// used as an endpoint for juju internal communication. If there are
// no suitable addresses, the empty string is returned.
func SelectInternalAddress(addresses []Address, machineLocal bool) string {
	index := bestAddressIndex(len(addresses), globalPreferIPv6, func(i int) Address {
		return addresses[i]
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return ""
	}
	return addresses[index].Value
}

// SelectInternalHostPort picks one HostPort from a slice that can be
// used as an endpoint for juju internal communication and returns it
// in its NetAddr form. If there are no suitable addresses, the empty
// string is returned.
func SelectInternalHostPort(hps []HostPort, machineLocal bool) string {
	index := bestAddressIndex(len(hps), globalPreferIPv6, func(i int) Address {
		return hps[i].Address
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return ""
	}
	return hps[index].NetAddr()
}

func publicMatch(addr Address, preferIPv6 bool) scopeMatch {
	switch addr.Scope {
	case ScopePublic:
		return mayPreferIPv6(addr, exactScope, preferIPv6)
	case ScopeCloudLocal, ScopeUnknown:
		return mayPreferIPv6(addr, fallbackScope, preferIPv6)
	}
	return invalidScope
}

// mayPreferIPv6 returns mismatchedTypeExactScope or
// mismatchedTypeFallbackScope (depending on originalScope) if addr's
// type is IPv4, and preferIPv6 is true. When preferIPv6 is false, or
// addr's type is IPv6 and preferIPv6 is true, returns the
// originalScope unchanged.
func mayPreferIPv6(addr Address, originalScope scopeMatch, preferIPv6 bool) scopeMatch {
	if preferIPv6 && addr.Type != IPv6Address {
		switch originalScope {
		case exactScope:
			return mismatchedTypeExactScope
		case fallbackScope:
			return mismatchedTypeFallbackScope
		}
		return invalidScope
	}
	return originalScope
}

func internalAddressMatcher(machineLocal bool) func(Address, bool) scopeMatch {
	if machineLocal {
		return cloudOrMachineLocalMatch
	}
	return cloudLocalMatch
}

func cloudLocalMatch(addr Address, preferIPv6 bool) scopeMatch {
	switch addr.Scope {
	case ScopeCloudLocal:
		return mayPreferIPv6(addr, exactScope, preferIPv6)
	case ScopePublic, ScopeUnknown:
		return mayPreferIPv6(addr, fallbackScope, preferIPv6)
	}
	return invalidScope
}

func cloudOrMachineLocalMatch(addr Address, preferIPv6 bool) scopeMatch {
	if addr.Scope == ScopeMachineLocal {
		return mayPreferIPv6(addr, exactScope, preferIPv6)
	}
	return cloudLocalMatch(addr, preferIPv6)
}

type scopeMatch int

const (
	invalidScope scopeMatch = iota
	exactScope
	fallbackScope
	mismatchedTypeExactScope
	mismatchedTypeFallbackScope
)

// bestAddressIndex returns the index of the first address
// with an exactly matching scope, or the first address with
// a matching fallback scope if there are no exact matches, or
// a matching scope but mismatched type when preferIPv6 is true.
// If there are no suitable addresses, -1 is returned.
func bestAddressIndex(numAddr int, preferIPv6 bool, getAddr func(i int) Address, match func(addr Address, preferIPv6 bool) scopeMatch) int {
	fallbackAddressIndex := -1
	mismatchedTypeFallbackIndex := -1
	mismatchedTypeExactIndex := -1
	for i := 0; i < numAddr; i++ {
		addr := getAddr(i)
		switch match(addr, preferIPv6) {
		case exactScope:
			logger.Tracef("exactScope match: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", i, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			return i
		case fallbackScope:
			logger.Tracef("fallbackScope match: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", i, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			// Use the first fallback address if there are no exact matches.
			if fallbackAddressIndex == -1 {
				fallbackAddressIndex = i
			}
		case mismatchedTypeExactScope:
			logger.Tracef("mismatchedTypeExactScope match: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", i, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			// We have an exact scope match, but the type does not
			// match, so save the first index as this is the best
			// match so far.
			if mismatchedTypeExactIndex == -1 {
				mismatchedTypeExactIndex = i
			}
		case mismatchedTypeFallbackScope:
			logger.Tracef("mismatchedTypeFallbackScope match: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", i, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			// We have a fallback scope match, but the type does not
			// match, so we save the first index in case this is the
			// best match so far.
			if mismatchedTypeFallbackIndex == -1 {
				mismatchedTypeFallbackIndex = i
			}
		}
	}
	if preferIPv6 {
		if fallbackAddressIndex != -1 {
			// Prefer an IPv6 fallback to a IPv4 mismatch.
			logger.Tracef("fallbackScope return: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", fallbackAddressIndex, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			return fallbackAddressIndex
		}
		if mismatchedTypeExactIndex != -1 {
			// Prefer an exact IPv4 match to a fallback.
			logger.Tracef("mismatchedTypeExactScope return: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", mismatchedTypeExactIndex, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
			return mismatchedTypeExactIndex
		}
		logger.Tracef("mismatchedTypeFallbackScope return: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", mismatchedTypeFallbackIndex, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
		return mismatchedTypeFallbackIndex
	}
	logger.Tracef("fallbackScope return: index=%d,fallback=%d,mismatchedExact=%d,mismatchedFallback=%d,preferIPv6=%v", fallbackAddressIndex, fallbackAddressIndex, mismatchedTypeExactIndex, mismatchedTypeFallbackIndex, preferIPv6)
	return fallbackAddressIndex
}

// sortOrder calculates the "weight" of the address when sorting,
// taking into account the preferIPv6 flag:
// - public IPs first;
// - hostnames after that, but "localhost" will be last if present;
// - cloud-local next;
// - machine-local next;
// - link-local next;
// - non-hostnames with unknown scope last.
//
// When preferIPv6 flag and the address type do not match, the order
// is incremented to put non-preferred addresses after preferred.
func (a Address) sortOrder(preferIPv6 bool) int {
	order := 0xFF
	switch a.Scope {
	case ScopePublic:
		order = 0x00
	case ScopeCloudLocal:
		order = 0x20
	case ScopeMachineLocal:
		order = 0x40
	case ScopeLinkLocal:
		order = 0x80
	}
	switch a.Type {
	case HostName:
		order = 0x10
		if a.Value == "localhost" {
			order++
		}
	case IPv4Address:
		if preferIPv6 {
			order++
		}
	case IPv6Address:
		if !preferIPv6 {
			order++
		}
	}
	return order
}

type addressesPreferringIPv4Slice []Address

func (a addressesPreferringIPv4Slice) Len() int      { return len(a) }
func (a addressesPreferringIPv4Slice) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a addressesPreferringIPv4Slice) Less(i, j int) bool {
	addr1 := a[i]
	addr2 := a[j]
	order1 := addr1.sortOrder(false)
	order2 := addr2.sortOrder(false)
	if order1 == order2 {
		return addr1.Value < addr2.Value
	}
	return order1 < order2
}

type addressesPreferringIPv6Slice struct {
	addressesPreferringIPv4Slice
}

func (a addressesPreferringIPv6Slice) Less(i, j int) bool {
	addr1 := a.addressesPreferringIPv4Slice[i]
	addr2 := a.addressesPreferringIPv4Slice[j]
	order1 := addr1.sortOrder(true)
	order2 := addr2.sortOrder(true)
	if order1 == order2 {
		return addr1.Value < addr2.Value
	}
	return order1 < order2
}

// SortAddresses sorts the given Address slice according to the
// sortOrder of each address and the preferIpv6 flag. See
// Address.sortOrder() for more info.
func SortAddresses(addrs []Address, preferIPv6 bool) {
	if preferIPv6 {
		sort.Sort(addressesPreferringIPv6Slice{addressesPreferringIPv4Slice(addrs)})
	} else {
		sort.Sort(addressesPreferringIPv4Slice(addrs))
	}
}

// DecimalToIPv4 converts a decimal to the dotted quad IP address format.
func DecimalToIPv4(addr uint32) net.IP {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, addr)
	return net.IP(bytes)
}

// IPv4ToDecimal converts a dotted quad IP address to its decimal equivalent.
func IPv4ToDecimal(ipv4Addr net.IP) (uint32, error) {
	ip := ipv4Addr.To4()
	if ip == nil {
		return 0, errors.Errorf("%q is not a valid IPv4 address", ipv4Addr.String())
	}
	return binary.BigEndian.Uint32([]byte(ip)), nil
}
