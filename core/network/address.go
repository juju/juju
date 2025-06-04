// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sort"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
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

// AddressConfigType defines valid network link configuration types.
// See interfaces(5) for details.
type AddressConfigType string

const (
	ConfigUnknown  AddressConfigType = ""
	ConfigDHCP     AddressConfigType = "dhcp"
	ConfigStatic   AddressConfigType = "static"
	ConfigManual   AddressConfigType = "manual"
	ConfigLoopback AddressConfigType = "loopback"
)

// IsValidAddressConfigType returns whether the given value is a valid
// method to configure a link-layer network device's IP address.
// TODO (manadart 2021-05-04): There is an issue with the usage of this
// method in state where we have denormalised the config method so it is
// against device addresses. This is because "manual" indicates a device that
// has no configuration by default. This could never apply to an address.
func IsValidAddressConfigType(value string) bool {
	switch AddressConfigType(value) {
	case ConfigLoopback, ConfigStatic, ConfigDHCP, ConfigManual:
		return true
	}
	return false
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

	// AddressCIDR returns the subnet CIDR of the address.
	AddressCIDR() string

	// AddressConfigType returns the configuration method of the address.
	AddressConfigType() AddressConfigType

	// AddressIsSecondary returns whether this address is not the
	// primary address associated with the network device.
	AddressIsSecondary() bool
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

// SortOrderOrigin calculates the "weight" of the address origin to use when
// sorting such that the most accessible addresses will appear first:
// - provider addresses first;
// - machine addresses next;
// - unknown addresses last.
func SortOrderOrigin(sas SpaceAddress) int {
	switch sas.Origin {
	case OriginProvider:
		return 0
	case OriginMachine:
		return 1
	case OriginUnknown:
		return 2
	}
	return 3
}

// SortOrderMostPublic calculates the "weight" of the address to use when
// sorting such that the most accessible addresses will appear first:
// - public IPs first;
// - hostnames after that, but "localhost" will be last if present;
// - cloud-local next;
// - fan-local next;
// - machine-local next;
// - link-local next;
// - non-hostnames with unknown scope last.
// Secondary addresses with otherwise equal weight will be sorted to come after
// primary addresses, including host names *except* localhost.
func SortOrderMostPublic(a Address) int {
	order := 100

	switch a.AddressScope() {
	case ScopePublic:
		order = 0
		// Special case to ensure that these follow non-localhost host names.
		if a.AddressIsSecondary() {
			order = 10
		}
	case ScopeCloudLocal:
		order = 30
	case ScopeFanLocal:
		order = 50
	case ScopeMachineLocal:
		order = 70
	case ScopeLinkLocal:
		order = 90
	}

	switch a.AddressType() {
	case HostName:
		order = 10
		if a.Host() == "localhost" {
			order = 20
		}
	case IPv6Address:
		order++
	case IPv4Address:
	}

	if a.AddressIsSecondary() {
		order += 2
	}

	return order
}

// MachineAddress represents an address without associated space or provider
// information. Addresses of this form will be supplied by an agent running
// directly on a machine or container, or returned for requests where space
// information is irrelevant to usage.
type MachineAddress struct {
	// Value is an IP address or hostname.
	Value string

	// Type indicates the form of the address value;
	// IPv4, IPv6 or host-name.
	Type AddressType

	// Scope indicates the visibility of this address.
	Scope Scope

	// CIDR is used for IP addresses to indicate
	// the subnet that they are part of.
	CIDR string

	// ConfigType denotes how this address was configured.
	ConfigType AddressConfigType

	// IsSecondary if true, indicates that this address is not the primary
	// address associated with the network device.
	IsSecondary bool
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

// AddressCIDR returns the subnet CIDR of the address.
func (a MachineAddress) AddressCIDR() string {
	return a.CIDR
}

// AddressConfigType returns the configuration method of the address.
func (a MachineAddress) AddressConfigType() AddressConfigType {
	return a.ConfigType
}

// AddressIsSecondary returns whether this address is not the
// primary address associated with the network device.
func (a MachineAddress) AddressIsSecondary() bool {
	return a.IsSecondary
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

// IP returns the net.IP representation of this address.
func (a MachineAddress) IP() net.IP {
	return net.ParseIP(a.Value)
}

// ValueWithMask returns the value of the address combined
// with the subnet mask indicated by its CIDR.
func (a MachineAddress) ValueWithMask() (string, error) {
	// Returning a NotFound error preserves prior behaviour from when
	// CIDRAddress was a method on InterfaceInfo.
	// TODO (manadart 2021-03-16): Rethink this as we clean up InterfaceInfos
	// and its corresponding wire type.
	if a.Value == "" || a.CIDR == "" {
		return "", errors.Errorf("address and CIDR pair (%q, %q) %w", a.Value, a.CIDR, coreerrors.NotFound)
	}

	_, ipNet, err := net.ParseCIDR(a.CIDR)
	if err != nil {
		return "", errors.Capture(err)
	}

	ip := a.IP()
	if ip == nil {
		return "", errors.Errorf("cannot parse IP address %q", a.Value)
	}

	ipNet.IP = ip
	return ipNet.String(), nil
}

// AsProviderAddress is used to construct a ProviderAddress
// from a MachineAddress
func (a MachineAddress) AsProviderAddress(options ...func(mutator ProviderAddressMutator)) ProviderAddress {
	addr := ProviderAddress{MachineAddress: a}

	for _, option := range options {
		option(&addr)
	}

	return addr
}

// NewMachineAddress creates a new MachineAddress,
// applying any supplied options to the result.
func NewMachineAddress(value string, options ...func(AddressMutator)) MachineAddress {
	addr := MachineAddress{
		Value: value,
		Type:  DeriveAddressType(value),
		Scope: ScopeUnknown,
	}

	for _, option := range options {
		option(&addr)
	}

	if addr.Scope == ScopeUnknown {
		addr.Scope = deriveScope(addr)
	}

	return addr
}

// MachineAddresses is a slice of MachineAddress
type MachineAddresses []MachineAddress

// NewMachineAddresses is a convenience function to create addresses
// from a variable number of string arguments, applying any supplied
// options to each address
func NewMachineAddresses(values []string, options ...func(AddressMutator)) MachineAddresses {
	if len(values) == 0 {
		return nil
	}

	addrs := make(MachineAddresses, len(values))
	for i, value := range values {
		addrs[i] = NewMachineAddress(value, options...)
	}
	return addrs
}

// AsProviderAddresses is used to construct ProviderAddresses
// element-wise from MachineAddresses
func (as MachineAddresses) AsProviderAddresses(options ...func(mutator ProviderAddressMutator)) ProviderAddresses {
	if len(as) == 0 {
		return nil
	}

	addrs := make(ProviderAddresses, len(as))
	for i, addr := range as {
		addrs[i] = addr.AsProviderAddress(options...)
	}
	return addrs
}

// AllMatchingScope returns the addresses that satisfy
// the input scope matching function.
func (as MachineAddresses) AllMatchingScope(getMatcher ScopeMatchFunc) MachineAddresses {
	return allMatchingScope(as, getMatcher)
}

// Values transforms the MachineAddresses to a string slice
// containing their raw IP values.
func (as MachineAddresses) Values() []string {
	return toStrings(as)
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

// InterfaceAddrs is patched for tests.
var InterfaceAddrs = func() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}

// IsLocalAddress returns true if the provided IP address equals to one of the
// local IP addresses.
func IsLocalAddress(ip net.IP) (bool, error) {
	addrs, err := InterfaceAddrs()
	if err != nil {
		return false, errors.Capture(err)
	}

	for _, addr := range addrs {
		localIP, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		if localIP.To4() != nil || localIP.To16() != nil {
			if ip.Equal(localIP) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ProviderAddress represents an address supplied by provider logic.
// It can include the provider's knowledge of the space in which the
// address resides.
type ProviderAddress struct {
	MachineAddress

	// SpaceName is the space in which this address resides
	SpaceName SpaceName

	// ProviderSpaceID is the provider's ID for the space this address is in
	ProviderSpaceID Id

	// ProviderID is the ID of this address's provider
	ProviderID Id

	// ProviderSubnetID is the provider's ID for the subnet this address is in
	ProviderSubnetID Id

	// ProviderVLANID is the provider's ID for the VLAN this address is in
	ProviderVLANID Id

	// VLANTag is the tag associated with this address's VLAN
	VLANTag int
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
	if a.ProviderSpaceID != "" {
		if !spaceFound {
			buf.WriteByte('@')
		}
		buf.WriteString(fmt.Sprintf("(id:%v)", string(a.ProviderSpaceID)))
	}

	return buf.String()
}

// ProviderAddresses is a slice of ProviderAddress
// supporting conversion to SpaceAddresses.
type ProviderAddresses []ProviderAddress

// Values transforms the ProviderAddresses to a string slice containing
// their raw IP values.
func (pas ProviderAddresses) Values() []string {
	return toStrings(pas)
}

// ToSpaceAddresses transforms the ProviderAddresses to SpaceAddresses by using
// the input lookup to get a space ID from the name or the CIDR.
func (pas ProviderAddresses) ToSpaceAddresses(spaceInfos SpaceInfos) (SpaceAddresses, error) {
	if pas == nil {
		return nil, nil
	}

	sas := make(SpaceAddresses, len(pas))
	for i, pa := range pas {
		sas[i] = SpaceAddress{MachineAddress: pa.MachineAddress}

		// If the provider explicitly sets the space, i.e. MAAS, prefer the name.
		if pa.SpaceName != "" {
			info := spaceInfos.GetByName(pa.SpaceName)
			if info == nil {
				return nil, errors.Errorf("space with name %q %w", pa.SpaceName, coreerrors.NotFound)
			}
			sas[i].SpaceID = info.ID
			continue
		}

		// Otherwise attempt to look up the CIDR.
		sInfo, err := spaceInfos.InferSpaceFromCIDRAndSubnetID(pa.CIDR, string(pa.ProviderSubnetID))
		if err != nil {
			logger.Debugf(context.TODO(), "no matching subnet for CIDR %q and provider ID %q", pa.CIDR, pa.ProviderSubnetID)
			continue
		}
		sas[i].SpaceID = sInfo.ID
	}
	return sas, nil
}

// OneMatchingScope returns the address that best satisfies the input scope
// matching function. The boolean return indicates if a match was found.
func (pas ProviderAddresses) OneMatchingScope(getMatcher ScopeMatchFunc) (ProviderAddress, bool) {
	indexes := indexesForScope(pas, getMatcher)
	if len(indexes) == 0 {
		return ProviderAddress{}, false
	}
	addr := pas[indexes[0]]
	logger.Debugf(context.TODO(), "selected %q as address, using scope %q", addr.Value, addr.Scope)
	return addr, true
}

// SpaceAddress represents the location of a machine, including metadata
// about what kind of location the address describes.
// This is a server-side type that may include a space reference.
// It is used in logic for filtering addresses by space.
type SpaceAddress struct {
	MachineAddress
	Origin  Origin
	SpaceID SpaceUUID
}

// GoString implements fmt.GoStringer.
func (a SpaceAddress) GoString() string {
	return a.String()
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
		buf.WriteString(a.SpaceID.String())
	}

	return buf.String()
}

// NewSpaceAddress creates a new SpaceAddress,
// applying any supplied options to the result.
func NewSpaceAddress(value string, options ...func(mutator AddressMutator)) SpaceAddress {
	return SpaceAddress{MachineAddress: NewMachineAddress(value, options...)}
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

// Values returns a slice of strings containing the IP/host-name of each of
// the receiver addresses.
func (sas SpaceAddresses) Values() []string {
	return toStrings(sas)
}

// ToProviderAddresses transforms the SpaceAddresses to ProviderAddresses by using
// the input lookup for conversion of space ID to space info.
func (sas SpaceAddresses) ToProviderAddresses(spaceInfos SpaceInfos) (ProviderAddresses, error) {
	if sas == nil {
		return nil, nil
	}

	pas := make(ProviderAddresses, len(sas))
	for i, sa := range sas {
		pas[i] = ProviderAddress{MachineAddress: sa.MachineAddress}
		if sa.SpaceID != "" {
			info := spaceInfos.GetByID(sa.SpaceID)
			if info == nil {
				return nil, errors.Errorf("space with ID %q %w", sa.SpaceID, coreerrors.NotFound)
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
		logger.Errorf(context.TODO(), "addresses not filtered - no spaces given.")
		return sas, false
	}

	spaceInfos := SpaceInfos(spaces)
	var selectedAddresses SpaceAddresses
	for _, addr := range sas {
		if space := spaceInfos.GetByID(addr.SpaceID); space != nil {
			logger.Debugf(context.TODO(), "selected %q as an address in space %q", addr.Value, space.Name)
			selectedAddresses = append(selectedAddresses, addr)
		}
	}

	if len(selectedAddresses) > 0 {
		return selectedAddresses, true
	}

	logger.Errorf(context.TODO(), "no addresses found in spaces %s", spaceInfos)
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

// AllMatchingScope returns the addresses that satisfy
// the input scope matching function.
func (sas SpaceAddresses) AllMatchingScope(getMatcher ScopeMatchFunc) SpaceAddresses {
	return allMatchingScope(sas, getMatcher)
}

// EqualTo returns true if this set of SpaceAddresses is equal to other.
func (sas SpaceAddresses) EqualTo(other SpaceAddresses) bool {
	if len(sas) != len(other) {
		return false
	}

	sort.Sort(sas)
	sort.Sort(other)
	for i := 0; i < len(sas); i++ {
		if sas[i].String() != other[i].String() {
			return false
		}
	}

	return true
}

func (sas SpaceAddresses) Len() int      { return len(sas) }
func (sas SpaceAddresses) Swap(i, j int) { sas[i], sas[j] = sas[j], sas[i] }
func (sas SpaceAddresses) Less(i, j int) bool {
	addr1 := sas[i]
	addr2 := sas[j]
	// Sort by scope first, then by origin then by address value.
	order1 := SortOrderMostPublic(addr1)
	order2 := SortOrderMostPublic(addr2)
	if order1 == order2 {
		originOrder1 := SortOrderOrigin(addr1)
		originOrder2 := SortOrderOrigin(addr2)
		if originOrder1 == originOrder2 {
			return addr1.Value < addr2.Value
		}
		return originOrder1 < originOrder2
	}
	return order1 < order2
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

// CIDRAddressType returns back an AddressType to indicate whether the supplied
// CIDR corresponds to an IPV4 or IPV6 range. An error will be returned if a
// non-valid CIDR is provided.
//
// Caveat: if the provided CIDR corresponds to an IPV6 range with a 4in6
// prefix, the function will classify it as an IPV4 address. This is a known
// limitation of the go stdlib IP parsing code but it's not something that we
// are likely to encounter in the wild so there is no need to add extra logic
// to work around it.
func CIDRAddressType(cidr string) (AddressType, error) {
	_, netIP, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	if netIP.IP.To4() != nil {
		return IPv4Address, nil
	}

	return IPv6Address, nil
}

// NetworkCIDRFromIPAndMask constructs a CIDR for a network by applying the
// provided netmask to the specified address (can be either a host or network
// address) and formatting the result as a CIDR.
//
// For example, passing 10.0.0.4 and a /24 mask yields 10.0.0.0/24.
func NetworkCIDRFromIPAndMask(ip net.IP, netmask net.IPMask) string {
	if ip == nil || netmask == nil {
		return ""
	}

	hostBits, _ := netmask.Size()
	return fmt.Sprintf("%s/%d", ip.Mask(netmask), hostBits)
}

// SpaceAddressCandidate describes property methods required
// for conversion to sortable space addresses.
type SpaceAddressCandidate interface {
	Value() string
	ConfigMethod() AddressConfigType
	SubnetCIDR() string
	IsSecondary() bool
}

// ConvertToSpaceAddress returns a SpaceAddress representing the
// input candidate address, by using the input subnet lookup to
// associate the address with a space..
func ConvertToSpaceAddress(addr SpaceAddressCandidate, lookup SubnetLookup) (SpaceAddress, error) {
	subnets, err := lookup.AllSubnetInfos()
	if err != nil {
		return SpaceAddress{}, errors.Capture(err)
	}

	cidr := addr.SubnetCIDR()

	spaceAddr := SpaceAddress{
		MachineAddress: NewMachineAddress(
			addr.Value(),
			WithCIDR(cidr),
			WithConfigType(addr.ConfigMethod()),
			WithSecondary(addr.IsSecondary()),
		),
	}

	// Attempt to set the space ID based on the subnet.
	if cidr != "" {
		allMatching, err := subnets.GetByCIDR(cidr)
		if err != nil {
			return SpaceAddress{}, errors.Capture(err)
		}

		// This only holds true while CIDRs uniquely identify subnets.
		if len(allMatching) != 0 {
			spaceAddr.SpaceID = allMatching[0].SpaceID
		}
	}

	return spaceAddr, nil
}

// noAddress represents an error when an address is requested but not available.
type noAddress struct {
	error
}

// NoAddressError returns an error which satisfies IsNoAddressError(). The given
// addressKind specifies what kind of address(es) is(are) missing, usually
// "private" or "public".
func NoAddressError(addressKind string) error {
	newErr := errors.Errorf("no %s address(es)", addressKind)
	return &noAddress{newErr}
}

// IsNoAddressError reports whether err was created with NoAddressError().
func IsNoAddressError(err error) bool {
	_, ok := err.(*noAddress)
	return ok
}

// toStrings returns the IP addresses in string form for input
// that is a slice of types implementing the Address interface.
func toStrings[T Address](addrs []T) []string {
	if addrs == nil {
		return nil
	}

	ips := make([]string, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.Host()
	}
	return ips
}

func allMatchingScope[T Address](addrs []T, getMatcher ScopeMatchFunc) []T {
	indexes := indexesForScope(addrs, getMatcher)
	if len(indexes) == 0 {
		return nil
	}
	out := make([]T, len(indexes))
	for i, index := range indexes {
		out[i] = addrs[index]
	}
	return out
}

// indexesForScope returns the indexes of the addresses with the best
// matching scope and type (according to the matchFunc).
// An empty slice is returned if there were no suitable addresses.
func indexesForScope[T Address](addrs []T, matchFunc ScopeMatchFunc) []int {
	matches := filterAndCollateAddressIndexes(addrs, matchFunc)

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
func indexesByScopeMatch[T Address](addrs []T, matchFunc ScopeMatchFunc) []int {
	matches := filterAndCollateAddressIndexes(addrs, matchFunc)

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
func filterAndCollateAddressIndexes[T Address](addrs []T, matchFunc ScopeMatchFunc) map[ScopeMatch][]int {
	matches := make(map[ScopeMatch][]int)
	for i, addr := range addrs {
		matchType := matchFunc(addr)
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
