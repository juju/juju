// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bytes"
	"fmt"
	"net"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
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

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ipNet
}

// AddressType represents the possible ways of specifying a machine location by
// either a hostname resolvable by dns lookup, or IPv4 or IPv6 address.
type AddressType string

const (
	HostName    AddressType = "hostname"
	IPv4Address AddressType = "ipv4"
	IPv6Address AddressType = "ipv6"
)

// Scope denotes the context a location may apply to. If a name or address can
// be reached from the wider internet, it is considered public.
// A private network address is either specific to the cloud or cloud subnet a
// machine belongs to, or to the machine itself for containers.
type Scope string

const (
	ScopeUnknown      Scope = ""
	ScopePublic       Scope = "public"
	ScopeCloudLocal   Scope = "local-cloud"
	ScopeFanLocal     Scope = "local-fan"
	ScopeMachineLocal Scope = "local-machine"
	ScopeLinkLocal    Scope = "link-local"
)

// ScopeMatch is a numeric designation of how well the requirement
// for satisfying a scope is met.
type ScopeMatch int

const (
	invalidScope ScopeMatch = iota
	exactScopeIPv4
	exactScope
	firstFallbackScopeIPv4
	firstFallbackScope
	secondFallbackScopeIPv4
	secondFallbackScope
)

// Address describes methods for returning details
// about an IP address or host name.
type Address interface {
	// Host returns the value for the host-name/IP address.
	Host() string

	// AddressType returns the type of the address.
	AddressType() AddressType

	// AddressScope returns the scope of the address.
	AddressScope() Scope
}

// ScopeMatchFunc is an alias for a function that accepts an Address,
// and returns what kind of scope match is determined by the body.
type ScopeMatchFunc = func(addr Address) ScopeMatch

// ExactScopeMatch checks if an address exactly
// matches any of the specified scopes.
func ExactScopeMatch(addr Address, addrScopes ...Scope) bool {
	for _, scope := range addrScopes {
		if addr.AddressScope() == scope {
			return true
		}
	}
	return false
}

// MachineAddress represents an address without associated space or provider
// information. Addresses of this form will be supplied by an agent running
// directly on a machine or container, or returned for requests where space
// information is irrelevant to usage.
type MachineAddress struct {
	Value string
	Type  AddressType
	Scope Scope
}

// Host returns the value for the host-name/IP address.
func (a MachineAddress) Host() string {
	return a.Value
}

// AddressType returns the type of the address.
func (a MachineAddress) AddressType() AddressType {
	return a.Type
}

// AddressScope returns the scope of the address.
func (a MachineAddress) AddressScope() Scope {
	return a.Scope
}

// GoString implements fmt.GoStringer.
func (a MachineAddress) GoString() string {
	return a.String()
}

// String returns the address value, prefixed with the scope if known.
func (a MachineAddress) String() string {
	var prefix string
	if a.Scope != ScopeUnknown {
		prefix = string(a.Scope) + ":"
	}
	return prefix + a.Value
}

// sortOrder calculates the "weight" of the address when sorting:
// - public IPs first;
// - hostnames after that, but "localhost" will be last if present;
// - cloud-local next;
// - fan-local next;
// - machine-local next;
// - link-local next;
// - non-hostnames with unknown scope last.
func (a MachineAddress) sortOrder() int {
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

// NewMachineAddress creates a new MachineAddress, deriving its type from the
// value and using ScopeUnknown as scope. It is a shortcut to calling
// NewScopedMachineAddress(value, ScopeUnknown).
func NewMachineAddress(value string) MachineAddress {
	return NewScopedMachineAddress(value, ScopeUnknown)
}

// NewScopedMachineAddress creates a new MachineAddress, deriving its type from
// the value.
// If the specified scope is ScopeUnknown, then NewScopedSpaceAddress will attempt
// to derive the scope based on reserved IP address ranges.
// Because passing ScopeUnknown is fairly common,
// NewMachineAddress() above does exactly that.
func NewScopedMachineAddress(value string, scope Scope) MachineAddress {
	addr := MachineAddress{
		Value: value,
		Type:  DeriveAddressType(value),
		Scope: scope,
	}
	if scope == ScopeUnknown {
		addr.Scope = deriveScope(addr)
	}
	return addr
}

// deriveScope attempts to derive the network scope from an address'
// type and value, returning the original network scope if no
// deduction can be made.
func deriveScope(addr MachineAddress) Scope {
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

// ProviderAddress represents an address supplied by provider logic.
// It can include the provider's knowledge of the space in which the
// address resides.
type ProviderAddress struct {
	MachineAddress
	SpaceName       SpaceName
	ProviderSpaceID Id
}

// GoString implements fmt.GoStringer.
func (a ProviderAddress) GoString() string {
	return a.String()
}

// String returns a string representation of the address, in the form:
// `<scope>:<address-value>@<space-name>(id:<space-provider-id)`; for example:
//
//	public:c2-54-226-162-124.compute-1.amazonaws.com@public-api(id:42)
//
// If the SpaceName is blank, the "@<space-name>" suffix will be omitted.
// Finally, if the ProviderSpaceID is empty the suffix
// "(id:<space-provider-id>)" part will be omitted as well.
func (a ProviderAddress) String() string {
	var buf bytes.Buffer
	buf.WriteString(a.MachineAddress.String())

	var spaceFound bool
	if a.SpaceName != "" {
		spaceFound = true
		buf.WriteByte('@')
		buf.WriteString(string(a.SpaceName))
	}
	if a.ProviderSpaceID != Id("") {
		if !spaceFound {
			buf.WriteByte('@')
		}
		buf.WriteString(fmt.Sprintf("(id:%v)", string(a.ProviderSpaceID)))
	}

	return buf.String()
}

// NewProviderAddress creates a new ProviderAddress, deriving its type from the
// value and using ScopeUnknown as scope. It is a shortcut to calling
// NewScopedProvider(value, ScopeUnknown).
func NewProviderAddress(value string) ProviderAddress {
	return ProviderAddress{MachineAddress: NewMachineAddress(value)}
}

// NewScopedProviderAddress creates a new ProviderAddress by embedding the
// result of NewScopedMachineAddress.
// No space information is populated.
func NewScopedProviderAddress(value string, scope Scope) ProviderAddress {
	return ProviderAddress{MachineAddress: NewScopedMachineAddress(value, scope)}
}

// NewProviderAddressInSpace creates a new ProviderAddress, deriving its type
// and scope from the value, and associating it with the given space name.
func NewProviderAddressInSpace(spaceName string, value string) ProviderAddress {
	return ProviderAddress{
		MachineAddress: NewMachineAddress(value),
		SpaceName:      SpaceName(spaceName),
	}
}

// NewScopedProviderAddressInSpace creates a new ProviderAddress, deriving its
// type from the value, and associating it with the given scope and space name.
func NewScopedProviderAddressInSpace(spaceName string, value string, scope Scope) ProviderAddress {
	return ProviderAddress{
		MachineAddress: NewScopedMachineAddress(value, scope),
		SpaceName:      SpaceName(spaceName),
	}
}

// ProviderAddresses is a slice of ProviderAddress
// supporting conversion to SpaceAddresses.
type ProviderAddresses []ProviderAddress

// NewProviderAddresses is a convenience function to create addresses
// from a variable number of string arguments.
func NewProviderAddresses(inAddresses ...string) (outAddresses ProviderAddresses) {
	outAddresses = make(ProviderAddresses, len(inAddresses))
	for i, address := range inAddresses {
		outAddresses[i] = NewProviderAddress(address)
	}
	return outAddresses
}

// NewProviderAddressesInSpace is a convenience function to create addresses
// in the same space, from a a variable number of string arguments.
func NewProviderAddressesInSpace(spaceName string, inAddresses ...string) (outAddresses ProviderAddresses) {
	outAddresses = make(ProviderAddresses, len(inAddresses))
	for i, address := range inAddresses {
		outAddresses[i] = NewProviderAddressInSpace(spaceName, address)
	}
	return outAddresses
}

// ToIPAddresses transforms the ProviderAddresses to a string slice containing
// their raw IP values.
func (pas ProviderAddresses) ToIPAddresses() []string {
	if pas == nil {
		return nil
	}

	ips := make([]string, len(pas))
	for i, addr := range pas {
		ips[i] = addr.Value
	}
	return ips
}

// ToSpaceAddresses transforms the ProviderAddresses to SpaceAddresses by using
// the input lookup for conversion of space name to space ID.
func (pas ProviderAddresses) ToSpaceAddresses(lookup SpaceLookup) (SpaceAddresses, error) {
	if pas == nil {
		return nil, nil
	}

	var spaceInfos SpaceInfos
	if len(pas) > 0 {
		var err error
		if spaceInfos, err = lookup.AllSpaceInfos(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	sas := make(SpaceAddresses, len(pas))
	for i, pa := range pas {
		sas[i] = SpaceAddress{MachineAddress: pa.MachineAddress}
		if pa.SpaceName != "" {
			info := spaceInfos.GetByName(string(pa.SpaceName))
			if info == nil {
				return nil, errors.NotFoundf("space with name %q", pa.SpaceName)
			}
			sas[i].SpaceID = info.ID
		}
	}
	return sas, nil
}

// OneMatchingScope returns the address that best satisfies the input scope
// matching function. The boolean return indicates if a match was found.
func (pas ProviderAddresses) OneMatchingScope(getMatcher ScopeMatchFunc) (ProviderAddress, bool) {
	indexes := indexesForScope(len(pas), func(i int) Address { return pas[i] }, getMatcher)
	if len(indexes) == 0 {
		return ProviderAddress{}, false
	}
	addr := pas[indexes[0]]
	logger.Debugf("selected %q as address, using scope %q", addr.Value, addr.Scope)
	return addr, true
}

// SpaceAddress represents the location of a machine, including metadata
// about what kind of location the address describes.
// This is a server-side type that may include a space reference.
// It is used in logic for filtering addresses by space.
type SpaceAddress struct {
	MachineAddress
	SpaceID string
}

// GoString implements fmt.GoStringer.
func (a SpaceAddress) GoString() string {
	return a.String()
}

// Converts the space address to a net.IP address assuming the space address is
// either v4 or v6. If the SpaceAddress value is not a valid ip address nil is
// returned
func (a SpaceAddress) IP() net.IP {
	return net.ParseIP(a.Value)
}

// String returns a string representation of the address, in the form:
// `<scope>:<address-value>@space:<space-id>`; for example:
//
//	public:c2-54-226-162-124.compute-1.amazonaws.com@space:1
//
// If the Space ID is empty, the @space:<space-id> suffix will be omitted.
func (a SpaceAddress) String() string {
	var buf bytes.Buffer
	buf.WriteString(a.MachineAddress.String())

	if a.SpaceID != "" {
		buf.WriteString("@space:")
		buf.WriteString(a.SpaceID)
	}

	return buf.String()
}

// NewSpaceAddress creates a new SpaceAddress, deriving its
// type from the input value and using ScopeUnknown as scope.
func NewSpaceAddress(value string) SpaceAddress {
	return NewScopedSpaceAddress(value, ScopeUnknown)
}

// NewScopedSpaceAddress creates a new SpaceAddress,
// deriving its type from the input value.
// If the specified scope is ScopeUnknown, then NewScopedSpaceAddress will
// attempt to derive the scope based on reserved IP address ranges.
// Because passing ScopeUnknown is fairly common, NewSpaceAddress() above
// does exactly that.
func NewScopedSpaceAddress(value string, scope Scope) SpaceAddress {
	return SpaceAddress{MachineAddress: NewScopedMachineAddress(value, scope)}
}

// SpaceAddresses is a slice of SpaceAddress
// supporting conversion to ProviderAddresses.
type SpaceAddresses []SpaceAddress

// NewSpaceAddresses is a convenience function to create addresses
// from a variable number of string arguments.
func NewSpaceAddresses(inAddresses ...string) (outAddresses SpaceAddresses) {
	outAddresses = make(SpaceAddresses, len(inAddresses))
	for i, address := range inAddresses {
		outAddresses[i] = NewSpaceAddress(address)
	}
	return outAddresses
}

// ToProviderAddresses transforms the SpaceAddresses to ProviderAddresses by using
// the input lookup for conversion of space ID to space info.
func (sas SpaceAddresses) ToProviderAddresses(lookup SpaceLookup) (ProviderAddresses, error) {
	if sas == nil {
		return nil, nil
	}

	var spaces SpaceInfos
	if len(sas) > 0 {
		var err error
		if spaces, err = lookup.AllSpaceInfos(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	pas := make(ProviderAddresses, len(sas))
	for i, sa := range sas {
		pas[i] = ProviderAddress{MachineAddress: sa.MachineAddress}
		if sa.SpaceID != "" {
			info := spaces.GetByID(sa.SpaceID)
			if info == nil {
				return nil, errors.NotFoundf("space with ID %q", sa.SpaceID)
			}
			pas[i].SpaceName = info.Name
			pas[i].ProviderSpaceID = info.ProviderId
		}
	}
	return pas, nil
}

// InSpaces returns the SpaceAddresses that are in the input spaces.
func (sas SpaceAddresses) InSpaces(spaces ...SpaceInfo) (SpaceAddresses, bool) {
	if len(spaces) == 0 {
		logger.Errorf("addresses not filtered - no spaces given.")
		return sas, false
	}

	spaceInfos := SpaceInfos(spaces)
	var selectedAddresses SpaceAddresses
	for _, addr := range sas {
		if space := spaceInfos.GetByID(addr.SpaceID); space != nil {
			logger.Debugf("selected %q as an address in space %q", addr.Value, space.Name)
			selectedAddresses = append(selectedAddresses, addr)
		}
	}

	if len(selectedAddresses) > 0 {
		return selectedAddresses, true
	}

	logger.Errorf("no addresses found in spaces %s", spaceInfos)
	return sas, false
}

// OneMatchingScope returns the address that best satisfies the input scope
// matching function. The boolean return indicates if a match was found.
func (sas SpaceAddresses) OneMatchingScope(getMatcher ScopeMatchFunc) (SpaceAddress, bool) {
	addrs := sas.AllMatchingScope(getMatcher)
	if len(addrs) == 0 {
		return SpaceAddress{}, false
	}
	return addrs[0], true
}

// AllMatchingScope returns the addresses that best satisfy the input scope
// matching function.
func (sas SpaceAddresses) AllMatchingScope(getMatcher ScopeMatchFunc) SpaceAddresses {
	indexes := indexesForScope(len(sas), func(i int) Address { return sas[i] }, getMatcher)
	if len(indexes) == 0 {
		return nil
	}
	out := make(SpaceAddresses, len(indexes))
	for i, index := range indexes {
		out[i] = sas[index]
	}
	return out
}

// EqualTo returns true if this set of SpaceAddresses is equal to other.
func (sas SpaceAddresses) EqualTo(other SpaceAddresses) bool {
	if len(sas) != len(other) {
		return false
	}

	SortAddresses(sas)
	SortAddresses(other)
	for i := 0; i < len(sas); i++ {
		if sas[i].String() != other[i].String() {
			return false
		}
	}

	return true
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

// ScopeMatchPublic is an address scope matching function for determining the
// extent to which the input address' scope satisfies a requirement for public
// accessibility.
func ScopeMatchPublic(addr Address) ScopeMatch {
	switch addr.AddressScope() {
	case ScopePublic:
		if addr.AddressType() == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	case ScopeCloudLocal:
		if addr.AddressType() == IPv4Address {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case ScopeFanLocal, ScopeUnknown:
		if addr.AddressType() == IPv4Address {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

func ScopeMatchMachineOrCloudLocal(addr Address) ScopeMatch {
	if addr.AddressScope() == ScopeMachineLocal {
		if addr.AddressType() == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	}
	return ScopeMatchCloudLocal(addr)
}

// ScopeMatchCloudLocal is an address scope matching function for determining
// the extent to which the input address' scope satisfies a requirement for
// accessibility from within the local cloud.
// Machine-only addresses do not satisfy this matcher.
func ScopeMatchCloudLocal(addr Address) ScopeMatch {
	switch addr.AddressScope() {
	case ScopeCloudLocal:
		if addr.AddressType() == IPv4Address {
			return exactScopeIPv4
		}
		return exactScope
	case ScopeFanLocal:
		if addr.AddressType() == IPv4Address {
			return firstFallbackScopeIPv4
		}
		return firstFallbackScope
	case ScopePublic, ScopeUnknown:
		if addr.AddressType() == IPv4Address {
			return secondFallbackScopeIPv4
		}
		return secondFallbackScope
	}
	return invalidScope
}

type addressByIndexFunc func(index int) Address

// indexesForScope returns the indexes of the addresses with the best
// matching scope and type (according to the matchFunc).
// An empty slice is returned if there were no suitable addresses.
func indexesForScope(numAddr int, getAddrFunc addressByIndexFunc, matchFunc ScopeMatchFunc) []int {
	matches := filterAndCollateAddressIndexes(numAddr, getAddrFunc, matchFunc)

	for _, matchType := range scopeMatchHierarchy() {
		indexes, ok := matches[matchType]
		if ok && len(indexes) > 0 {
			return indexes
		}
	}
	return nil
}

// indexesByScopeMatch filters address indexes by matching scope,
// then returns them in descending order of best match.
func indexesByScopeMatch(numAddr int, getAddrFunc addressByIndexFunc, matchFunc ScopeMatchFunc) []int {
	matches := filterAndCollateAddressIndexes(numAddr, getAddrFunc, matchFunc)

	var prioritized []int
	for _, matchType := range scopeMatchHierarchy() {
		indexes, ok := matches[matchType]
		if ok && len(indexes) > 0 {
			prioritized = append(prioritized, indexes...)
		}
	}
	return prioritized
}

// filterAndCollateAddressIndexes filters address indexes using the input scope
// matching function, then returns the results grouped by scope match quality.
// Invalid results are omitted.
func filterAndCollateAddressIndexes(
	numAddr int, getAddrFunc addressByIndexFunc, matchFunc ScopeMatchFunc,
) map[ScopeMatch][]int {
	matches := make(map[ScopeMatch][]int)
	for i := 0; i < numAddr; i++ {
		matchType := matchFunc(getAddrFunc(i))
		if matchType != invalidScope {
			matches[matchType] = append(matches[matchType], i)
		}
	}
	return matches
}

func scopeMatchHierarchy() []ScopeMatch {
	return []ScopeMatch{
		exactScopeIPv4, exactScope,
		firstFallbackScopeIPv4, firstFallbackScope,
		secondFallbackScopeIPv4, secondFallbackScope,
	}
}

type addressesPreferringIPv4Slice []SpaceAddress

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
func SortAddresses(addrs []SpaceAddress) {
	sort.Sort(addressesPreferringIPv4Slice(addrs))
}

// MergedAddresses provides a single list of addresses without duplicates
// suitable for returning as an address list for a machine.
// TODO (cherylj) Add explicit unit tests - tracked with bug #1544158
func MergedAddresses(machineAddresses, providerAddresses []SpaceAddress) []SpaceAddress {
	merged := make([]SpaceAddress, 0, len(providerAddresses)+len(machineAddresses))
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

// IPToCIDRNotation receives as input an IP and a CIDR value and returns back
// the IP in CIDR notation.
func IPToCIDRNotation(ip, cidr string) (string, error) {
	_, netIP, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	netIP.IP = net.ParseIP(ip)
	return netIP.String(), nil
}
