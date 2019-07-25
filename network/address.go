// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corenetwork "github.com/juju/juju/core/network"
)

// Private and special use network ranges for IPv4 and IPv6.
// See: http://tools.ietf.org/html/rfc1918
// Also: http://tools.ietf.org/html/rfc4193
// And: https://tools.ietf.org/html/rfc6890
var (
	classAPrivate   = mustParseCIDR("10.0.0.0/8")
	classBPrivate   = mustParseCIDR("172.16.0.0/12")
	classCPrivate   = mustParseCIDR("192.168.0.0/16")
	ipv6UniqueLocal = mustParseCIDR("fc00::/7")
	classEReserved  = mustParseCIDR("240.0.0.0/4")
)

const (
	// LoopbackIPv4CIDR is the loopback CIDR range for IPv4.
	LoopbackIPv4CIDR = "127.0.0.0/8"

	// LoopbackIPv6CIDR is the loopback CIDR range for IPv6.
	LoopbackIPv6CIDR = "::1/128"
)

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

// SpaceName holds the Juju space name of an address.
type SpaceName string
type spaceNameList []SpaceName

func (s spaceNameList) String() string {
	namesString := make([]string, len(s))
	for i, v := range s {
		namesString[i] = string(v)
	}

	return strings.Join(namesString, ", ")
}

func (s spaceNameList) IndexOf(name SpaceName) int {
	for i := range s {
		if s[i] == name {
			return i
		}
	}
	return -1
}

const (
	ScopeUnknown      Scope = ""
	ScopePublic       Scope = "public"
	ScopeCloudLocal   Scope = "local-cloud"
	ScopeFanLocal     Scope = "local-fan"
	ScopeMachineLocal Scope = "local-machine"
	ScopeLinkLocal    Scope = "link-local"
)

// Address represents the location of a machine, including metadata
// about what kind of location the address describes.
type Address struct {
	Value string
	Type  AddressType
	Scope
	SpaceName
	SpaceProviderId corenetwork.Id
}

// String returns a string representation of the address, in the form:
// `<scope>:<address-value>@<space-name>(id:<space-provider-id)`; for example:
//
//	public:c2-54-226-162-124.compute-1.amazonaws.com@public-api(id:42)
//
// If the scope is ScopeUnknown, the initial "<scope>:" prefix will be omitted.
// If the SpaceName is blank, the "@<space-name>" suffix will be omitted.
// Finally, if the SpaceProviderId is empty the suffix
// "(id:<space-provider-id>)" part will be omitted as well.
func (a Address) String() string {
	var buf bytes.Buffer
	if a.Scope != ScopeUnknown {
		buf.WriteString(string(a.Scope))
		buf.WriteByte(':')
	}
	buf.WriteString(a.Value)

	var spaceFound bool
	if a.SpaceName != "" {
		spaceFound = true
		buf.WriteByte('@')
		buf.WriteString(string(a.SpaceName))
	}
	if a.SpaceProviderId != corenetwork.Id("") {
		if !spaceFound {
			buf.WriteByte('@')
		}
		buf.WriteString(fmt.Sprintf("(id:%v)", string(a.SpaceProviderId)))
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

// NewAddressOnSpace creates a new Address, deriving its type and scope from the
// value and associating it with the given spaceName.
func NewAddressOnSpace(spaceName string, value string) Address {
	addr := NewAddress(value)
	addr.SpaceName = SpaceName(spaceName)
	return addr
}

// NewAddresses is a convenience function to create addresses from a variable
// number of string arguments.
func NewAddresses(inAddresses ...string) (outAddresses []Address) {
	outAddresses = make([]Address, len(inAddresses))
	for i, address := range inAddresses {
		outAddresses[i] = NewAddress(address)
	}
	return outAddresses
}

// NewAddressesOnSpace is a convenience function to create addresses on the same
// space, from a a variable number of string arguments.
func NewAddressesOnSpace(spaceName string, inAddresses ...string) (outAddresses []Address) {
	outAddresses = make([]Address, len(inAddresses))
	for i, address := range inAddresses {
		outAddresses[i] = NewAddressOnSpace(spaceName, address)
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

func isIPv4ReservedEAddress(addrType AddressType, ip net.IP) bool {
	if addrType != IPv4Address {
		return false
	}
	return classEReserved.Contains(ip)
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
	if isIPv4ReservedEAddress(addr.Type, ip) {
		return ScopeFanLocal
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

// ExactScopeMatch checks if an address exactly matches any of the specified
// scopes.
func ExactScopeMatch(addr Address, addrScopes ...Scope) bool {
	for _, scope := range addrScopes {
		if addr.Scope == scope {
			return true
		}
	}
	return false
}

// SelectAddressesBySpaceNames filters the input slice of Addresses down to
// those in the input space names.
func SelectAddressesBySpaceNames(addresses []Address, spaceNames ...SpaceName) ([]Address, bool) {
	if len(spaceNames) == 0 {
		logger.Errorf("addresses not filtered - no spaces given.")
		return addresses, false
	}

	var selectedAddresses []Address
	for _, addr := range addresses {
		if spaceNameList(spaceNames).IndexOf(addr.SpaceName) >= 0 {
			logger.Debugf("selected %q as an address in space %q", addr.Value, addr.SpaceName)
			selectedAddresses = append(selectedAddresses, addr)
		}
	}

	if len(selectedAddresses) > 0 {
		return selectedAddresses, true
	}

	logger.Errorf("no addresses found in spaces %s", spaceNames)
	return addresses, false
}

// SelectHostPortsBySpaceNames filters the input slice of HostPorts down to
// those in the input space names.
func SelectHostPortsBySpaceNames(hps []HostPort, spaceNames ...SpaceName) ([]HostPort, bool) {
	if len(spaceNames) == 0 {
		logger.Errorf("host ports not filtered - no spaces given.")
		return hps, false
	}

	var selectedHostPorts []HostPort
	for _, hp := range hps {
		if spaceNameList(spaceNames).IndexOf(hp.SpaceName) >= 0 {
			logger.Debugf("selected %q as a hostPort in space %q", hp.Value, hp.SpaceName)
			selectedHostPorts = append(selectedHostPorts, hp)
		}
	}

	if len(selectedHostPorts) > 0 {
		return selectedHostPorts, true
	}

	logger.Errorf("no hostPorts found in spaces %s", spaceNames)
	return hps, false
}

// SelectControllerAddress returns the most suitable address to use as a Juju
// Controller (API/state server) endpoint given the list of addresses.
// The second return value is false when no address can be returned.
// When machineLocal is true both ScopeCloudLocal and ScopeMachineLocal
// addresses are considered during the selection, otherwise just ScopeCloudLocal are.
func SelectControllerAddress(addresses []Address, machineLocal bool) (Address, bool) {
	internalAddress, ok := SelectInternalAddress(addresses, machineLocal)
	logger.Debugf(
		"selected %q as controller address, using scope %q",
		internalAddress.Value, internalAddress.Scope,
	)
	return internalAddress, ok
}

// SelectPublicAddress picks one address from a slice that would be
// appropriate to display as a publicly accessible endpoint. If there
// are no suitable addresses, then ok is false (and an empty address is
// returned). If a suitable address is then ok is true.
func SelectPublicAddress(addresses []Address) (Address, bool) {
	index := bestAddressIndex(len(addresses), func(i int) Address {
		return addresses[i]
	}, publicMatch)
	if index < 0 {
		return Address{}, false
	}
	return addresses[index], true
}

// SelectPublicHostPort picks one HostPort from a slice that would be
// appropriate to display as a publicly accessible endpoint. If there
// are no suitable candidates, the empty string is returned.
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
// used as an endpoint for juju internal communication. If there are
// no suitable addresses, then ok is false (and an empty address is
// returned). If a suitable address was found then ok is true.
func SelectInternalAddress(addresses []Address, machineLocal bool) (Address, bool) {
	index := bestAddressIndex(len(addresses), func(i int) Address {
		return addresses[i]
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return Address{}, false
	}
	return addresses[index], true
}

// SelectInternalAddresses picks the best addresses from a slice that can be
// used as an endpoint for juju internal communication.
// I nil slice is returned if there are no suitable addresses identified.
func SelectInternalAddresses(addresses []Address, machineLocal bool) []Address {
	indexes := bestAddressIndexes(len(addresses), func(i int) Address {
		return addresses[i]
	}, internalAddressMatcher(machineLocal))
	if len(indexes) == 0 {
		return nil
	}

	out := make([]Address, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, addresses[index])
	}
	return out
}

// SelectInternalHostPort picks one HostPort from a slice that can be
// used as an endpoint for juju internal communication and returns it
// in its NetAddr form. If there are no suitable addresses, the empty
// string is returned.
func SelectInternalHostPort(hps []HostPort, machineLocal bool) string {
	index := bestAddressIndex(len(hps), func(i int) Address {
		return hps[i].Address
	}, internalAddressMatcher(machineLocal))
	if index < 0 {
		return ""
	}
	return hps[index].NetAddr()
}

// SelectInternalHostPorts picks the best matching HostPorts from a
// slice that can be used as an endpoint for juju internal
// communication and returns them in NetAddr form. If there are no
// suitable addresses, an empty slice is returned.
func SelectInternalHostPorts(hps []HostPort, machineLocal bool) []string {
	indexes := bestAddressIndexes(len(hps), func(i int) Address {
		return hps[i].Address
	}, internalAddressMatcher(machineLocal))

	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, hps[index].NetAddr())
	}
	return out
}

// PrioritizeInternalHostPorts orders the provided addresses by best
// match for use as an endpoint for juju internal communication and
// returns them in NetAddr form. If there are no suitable addresses
// then an empty slice is returned.
func PrioritizeInternalHostPorts(hps []HostPort, machineLocal bool) []string {
	indexes := prioritizedAddressIndexes(len(hps), func(i int) Address {
		return hps[i].Address
	}, internalAddressMatcher(machineLocal))

	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, hps[index].NetAddr())
	}
	return out
}

func publicMatch(addr Address) scopeMatch {
	switch addr.Scope {
	case ScopePublic:
		if addr.Type == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	case ScopeCloudLocal:
		if addr.Type == IPv4Address {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case ScopeFanLocal, ScopeUnknown:
		if addr.Type == IPv4Address {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

func internalAddressMatcher(machineLocal bool) scopeMatchFunc {
	if machineLocal {
		return cloudOrMachineLocalMatch
	}
	return cloudLocalMatch
}

func cloudLocalMatch(addr Address) scopeMatch {
	switch addr.Scope {
	case ScopeCloudLocal:
		if addr.Type == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	case ScopeFanLocal:
		if addr.Type == IPv4Address {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case ScopePublic, ScopeUnknown:
		if addr.Type == IPv4Address {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

func cloudOrMachineLocalMatch(addr Address) scopeMatch {
	if addr.Scope == ScopeMachineLocal {
		if addr.Type == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	}
	return cloudLocalMatch(addr)
}

type scopeMatch int

const (
	invalidScope scopeMatch = iota
	exactScopeIPv4
	exactScope
	firstFallbackScopeIPv4
	firstFallbackScope
	secondFallbackScopeIPv4
	secondFallbackScope
)

type scopeMatchFunc func(addr Address) scopeMatch

type addressByIndexFunc func(index int) Address

// bestAddressIndex returns the index of the addresses with the best matching
// scope (according to the matchFunc). -1 is returned if there were no suitable
// addresses.
func bestAddressIndex(numAddr int, getAddrFunc addressByIndexFunc, matchFunc scopeMatchFunc) int {
	indexes := bestAddressIndexes(numAddr, getAddrFunc, matchFunc)
	if len(indexes) > 0 {
		return indexes[0]
	}
	return -1
}

// bestAddressIndexes returns the indexes of the addresses with the best
// matching scope and type (according to the matchFunc). An empty slice is
// returned if there were no suitable addresses.
func bestAddressIndexes(numAddr int, getAddrFunc addressByIndexFunc, matchFunc scopeMatchFunc) []int {
	// Categorise addresses by scope and type matching quality.
	matches := filterAndCollateAddressIndexes(numAddr, getAddrFunc, matchFunc)

	// Retrieve the indexes of the addresses with the best scope and type match.
	allowedMatchTypes := []scopeMatch{exactScopeIPv4, exactScope, firstFallbackScopeIPv4, firstFallbackScope, secondFallbackScopeIPv4, secondFallbackScope}
	for _, matchType := range allowedMatchTypes {
		indexes, ok := matches[matchType]
		if ok && len(indexes) > 0 {
			return indexes
		}
	}
	return []int{}
}

func prioritizedAddressIndexes(numAddr int, getAddrFunc addressByIndexFunc, matchFunc scopeMatchFunc) []int {
	// Categorise addresses by scope and type matching quality.
	matches := filterAndCollateAddressIndexes(numAddr, getAddrFunc, matchFunc)

	// Retrieve the indexes of the addresses with the best scope and type match.
	allowedMatchTypes := []scopeMatch{exactScopeIPv4, exactScope, firstFallbackScopeIPv4, firstFallbackScope, secondFallbackScopeIPv4, secondFallbackScope}
	var prioritized []int
	for _, matchType := range allowedMatchTypes {
		indexes, ok := matches[matchType]
		if ok && len(indexes) > 0 {
			prioritized = append(prioritized, indexes...)
		}
	}
	return prioritized
}

func filterAndCollateAddressIndexes(numAddr int, getAddrFunc addressByIndexFunc, matchFunc scopeMatchFunc) map[scopeMatch][]int {
	// Categorise addresses by scope and type matching quality.
	matches := make(map[scopeMatch][]int)
	for i := 0; i < numAddr; i++ {
		matchType := matchFunc(getAddrFunc(i))
		switch matchType {
		case exactScopeIPv4, exactScope, firstFallbackScopeIPv4, firstFallbackScope, secondFallbackScopeIPv4, secondFallbackScope:
			matches[matchType] = append(matches[matchType], i)
		}
	}
	return matches
}

// sortOrder calculates the "weight" of the address when sorting:
// - public IPs first;
// - hostnames after that, but "localhost" will be last if present;
// - cloud-local next;
// - fan-local next;
// - machine-local next;
// - link-local next;
// - non-hostnames with unknown scope last.
func (a Address) sortOrder() int {
	order := 0xFF
	switch a.Scope {
	case ScopePublic:
		order = 0x00
	case ScopeCloudLocal:
		order = 0x20
	case ScopeFanLocal:
		order = 0x40
	case ScopeMachineLocal:
		order = 0x80
	case ScopeLinkLocal:
		order = 0xA0
	}
	switch a.Type {
	case HostName:
		order = 0x10
		if a.Value == "localhost" {
			order++
		}
	case IPv6Address:
		// Prefer IPv4 over IPv6 addresses.
		order++
	case IPv4Address:
	}
	return order
}

type addressesPreferringIPv4Slice []Address

func (a addressesPreferringIPv4Slice) Len() int      { return len(a) }
func (a addressesPreferringIPv4Slice) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a addressesPreferringIPv4Slice) Less(i, j int) bool {
	addr1 := a[i]
	addr2 := a[j]
	order1 := addr1.sortOrder()
	order2 := addr2.sortOrder()
	if order1 == order2 {
		return addr1.Value < addr2.Value
	}
	return order1 < order2
}

// SortAddresses sorts the given Address slice according to the sortOrder of
// each address. See Address.sortOrder() for more info.
func SortAddresses(addrs []Address) {
	sort.Sort(addressesPreferringIPv4Slice(addrs))
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

// ResolvableHostnames returns the set of all DNS resolvable names
// from addrs. Note that 'localhost' is always considered resolvable
// because it can be used both as an IPv4 or IPv6 endpoint (e.g., in
// IPv6-only networks).
func ResolvableHostnames(addrs []Address) []Address {
	resolvableAddrs := make([]Address, 0, len(addrs))
	for _, addr := range addrs {
		if addr.Value == "localhost" || net.ParseIP(addr.Value) != nil {
			resolvableAddrs = append(resolvableAddrs, addr)
			continue
		}
		_, err := netLookupIP(addr.Value)
		if err != nil {
			logger.Infof("removing unresolvable address %q: %v", addr.Value, err)
			continue
		}
		resolvableAddrs = append(resolvableAddrs, addr)
	}
	return resolvableAddrs
}

// MergedAddresses provides a single list of addresses without duplicates
// suitable for returning as an address list for a machine.
// TODO (cherylj) Add explicit unit tests - tracked with bug #1544158
func MergedAddresses(machineAddresses, providerAddresses []Address) []Address {
	merged := make([]Address, 0, len(providerAddresses)+len(machineAddresses))
	providerValues := set.NewStrings()
	for _, address := range providerAddresses {
		// Older versions of Juju may have stored an empty address so ignore it here.
		if address.Value == "" || providerValues.Contains(address.Value) {
			continue
		}
		providerValues.Add(address.Value)
		merged = append(merged, address)
	}
	for _, address := range machineAddresses {
		if !providerValues.Contains(address.Value) {
			merged = append(merged, address)
		}
	}
	return merged
}
