// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
)

var logger = loggo.GetLogger("juju.network")

// SpaceInvalidChars is a regexp for validating that space names contain no
// invalid characters.
var SpaceInvalidChars = regexp.MustCompile("[^0-9a-z-]")

// noAddress represents an error when an address is requested but not available.
type noAddress struct {
	errors.Err
}

// NoAddressError returns an error which satisfies IsNoAddressError(). The given
// addressKind specifies what kind of address(es) is(are) missing, usually
// "private" or "public".
func NoAddressError(addressKind string) error {
	newErr := errors.NewErr("no %s address(es)", addressKind)
	newErr.SetLocation(1)
	return &noAddress{newErr}
}

// IsNoAddressError reports whether err was created with NoAddressError().
func IsNoAddressError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*noAddress)
	return ok
}

// Id defines a provider-specific network id.
type Id string

// AnySubnet when passed as a subnet id should be interpreted by the
// providers as "the subnet id does not matter". It's up to the
// provider how to handle this case - it might return an error.
const AnySubnet Id = ""

// UnknownId can be used whenever an Id is needed but not known.
const UnknownId = ""

// DefaultLXDBridge is the bridge that gets used for LXD containers
const DefaultLXDBridge = "lxdbr0"

var dashPrefix = regexp.MustCompile("^-*")
var dashSuffix = regexp.MustCompile("-*$")
var multipleDashes = regexp.MustCompile("--+")

// ConvertSpaceName converts names between provider space names and valid juju
// space names.
// TODO(mfoord): once MAAS space name rules are in sync with juju space name
// rules this can go away.
func ConvertSpaceName(name string, existing set.Strings) string {
	// First lower case and replace spaces with dashes.
	name = strings.Replace(name, " ", "-", -1)
	name = strings.ToLower(name)
	// Replace any character that isn't in the set "-", "a-z", "0-9".
	name = SpaceInvalidChars.ReplaceAllString(name, "")
	// Get rid of any dashes at the start as that isn't valid.
	name = dashPrefix.ReplaceAllString(name, "")
	// And any at the end.
	name = dashSuffix.ReplaceAllString(name, "")
	// Repleace multiple dashes with a single dash.
	name = multipleDashes.ReplaceAllString(name, "-")
	// Special case of when the space name was only dashes or invalid
	// characters!
	if name == "" {
		name = "empty"
	}
	// If this name is in use add a numerical suffix.
	if existing.Contains(name) {
		counter := 2
		for existing.Contains(name + fmt.Sprintf("-%d", counter)) {
			counter += 1
		}
		name = name + fmt.Sprintf("-%d", counter)
	}
	return name
}

// SubnetInfo describes the bare minimum information for a subnet,
// which the provider knows about but juju might not yet.
type SubnetInfo struct {
	// CIDR of the network, in 123.45.67.89/24 format. Can be empty if
	// unknown.
	CIDR string

	// ProviderId is a provider-specific network id. This the only
	// required field.
	ProviderId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard, and used
	// to define a VLAN network. For more information, see:
	// http://en.wikipedia.org/wiki/IEEE_802.1Q.
	VLANTag int

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceProviderId holds the provider Id of the space associated with
	// this subnet. Can be empty if not supported.
	SpaceProviderId Id
}

type SpaceInfo struct {
	Name       string
	ProviderId Id
	Subnets    []SubnetInfo
}
type BySpaceName []SpaceInfo

func (s BySpaceName) Len() int      { return len(s) }
func (s BySpaceName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s BySpaceName) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

// InterfaceConfigType defines valid network interface configuration
// types. See interfaces(5) for details
type InterfaceConfigType string

const (
	ConfigUnknown  InterfaceConfigType = ""
	ConfigDHCP     InterfaceConfigType = "dhcp"
	ConfigStatic   InterfaceConfigType = "static"
	ConfigManual   InterfaceConfigType = "manual"
	ConfigLoopback InterfaceConfigType = "loopback"
)

// InterfaceType defines valid network interface types.
type InterfaceType string

const (
	UnknownInterface    InterfaceType = ""
	LoopbackInterface   InterfaceType = "loopback"
	EthernetInterface   InterfaceType = "ethernet"
	VLAN_8021QInterface InterfaceType = "802.1q"
	BondInterface       InterfaceType = "bond"
	BridgeInterface     InterfaceType = "bridge"
)

// InterfaceInfo describes a single network interface available on an
// instance. For providers that support networks, this will be
// available at StartInstance() time.
// TODO(mue): Rename to InterfaceConfig due to consistency later.
type InterfaceInfo struct {
	// DeviceIndex specifies the order in which the network interface
	// appears on the host. The primary interface has an index of 0.
	DeviceIndex int

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// ProviderSubnetId is the provider-specific id for the associated
	// subnet.
	ProviderSubnetId Id

	// ProviderSpaceId is the provider-specific id for the associated space, if
	// known and supported.
	ProviderSpaceId Id

	// ProviderVLANId is the provider-specific id of the VLAN for this
	// interface.
	ProviderVLANId Id

	// ProviderAddressId is the provider-specific id of the assigned address.
	ProviderAddressId Id

	// AvailabilityZones describes the availability zones the associated
	// subnet is in.
	AvailabilityZones []string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// ParentInterfaceName is the name of the parent interface to use, if known.
	ParentInterfaceName string

	// InterfaceType is the type of the interface.
	InterfaceType InterfaceType

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it.
	Disabled bool

	// NoAutoStart is true when the interface should not be configured
	// to start automatically on boot. By default and for
	// backwards-compatibility, interfaces are configured to
	// auto-start.
	NoAutoStart bool

	// ConfigType determines whether the interface should be
	// configured via DHCP, statically, manually, etc. See
	// interfaces(5) for more information.
	ConfigType InterfaceConfigType

	// Address contains an optional static IP address to configure for
	// this network interface. The subnet mask to set will be inferred
	// from the CIDR value.
	Address Address

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers []Address

	// MTU is the Maximum Transmission Unit controlling the maximum size of the
	// protocol packats that the interface can pass through. It is only used
	// when > 0.
	MTU int

	// DNSSearchDomains contains the default DNS domain to use for non-FQDN
	// lookups.
	DNSSearchDomains []string

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually is (one of) the host address(es).
	GatewayAddress Address
}

type interfaceInfoSlice []InterfaceInfo

func (s interfaceInfoSlice) Len() int      { return len(s) }
func (s interfaceInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s interfaceInfoSlice) Less(i, j int) bool {
	iface1 := s[i]
	iface2 := s[j]
	return iface1.DeviceIndex < iface2.DeviceIndex
}

// SortInterfaceInfo sorts a slice of InterfaceInfo on DeviceIndex in ascending
// order.
func SortInterfaceInfo(interfaces []InterfaceInfo) {
	sort.Sort(interfaceInfoSlice(interfaces))
}

// ActualInterfaceName returns raw interface name for raw interface (e.g. "eth0") and
// virtual interface name for virtual interface (e.g. "eth0.42")
func (i *InterfaceInfo) ActualInterfaceName() string {
	if i.VLANTag > 0 {
		return fmt.Sprintf("%s.%d", i.InterfaceName, i.VLANTag)
	}
	return i.InterfaceName
}

// IsVirtual returns true when the interface is a virtual device, as
// opposed to a physical device (e.g. a VLAN or a network alias)
func (i *InterfaceInfo) IsVirtual() bool {
	return i.VLANTag > 0
}

// IsVLAN returns true when the interface is a VLAN interface.
func (i *InterfaceInfo) IsVLAN() bool {
	return i.VLANTag > 0
}

// CIDRAddress returns Address.Value combined with CIDR mask.
func (i *InterfaceInfo) CIDRAddress() string {
	if i.CIDR == "" || i.Address.Value == "" {
		return ""
	}
	_, ipNet, err := net.ParseCIDR(i.CIDR)
	if err != nil {
		return errors.Trace(err).Error()
	}
	ip := net.ParseIP(i.Address.Value)
	if ip == nil {
		return errors.Errorf("cannot parse IP address %q", i.Address.Value).Error()
	}
	ipNet.IP = ip
	return ipNet.String()
}

// ProviderInterfaceInfo holds enough information to identify an
// interface or link layer device to a provider so that it can be
// queried or manipulated. Its initial purpose is to pass to
// provider.ReleaseContainerAddresses.
type ProviderInterfaceInfo struct {
	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string
}

// DeviceToBridge gives the information about a particular device that
// should be bridged.
type DeviceToBridge struct {
	// DeviceName is the name of the device on the machine that should
	// be bridged.
	DeviceName string

	// BridgeName is the name of the bride that we want created.
	BridgeName string
}

// LXCNetDefaultConfig is the location of the default network config
// of the lxc package. It's exported to allow cross-package testing.
var LXCNetDefaultConfig = "/etc/default/lxc-net"

// InterfaceByNameAddrs returns the addresses for the given interface
// name. It's exported to facilitate cross-package testing.
var InterfaceByNameAddrs = func(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}

// filterAddrs looks at all of the addresses in allAddresses and removes ones
// that line up with removeAddresses. Note that net.Addr may be just an IP or
// may be a CIDR.
func filterAddrs(bridgeName string, allAddresses []Address, removeAddresses []net.Addr) []Address {
	filtered := make([]Address, 0, len(allAddresses))
	// TODO(jam) ips could be turned into a map[string]bool rather than
	// iterating over all of them, as we only compare against ip.String()
	ips := make([]net.IP, 0, len(removeAddresses))
	ipNets := make([]*net.IPNet, 0, len(removeAddresses))
	for _, ifaceAddr := range removeAddresses {
		// First check if this is a CIDR, as
		// net.InterfaceAddrs might return this instead of
		// a plain IP.
		ip, ipNet, err := net.ParseCIDR(ifaceAddr.String())
		if err != nil {
			// It's not a CIDR, try parsing as IP.
			ip = net.ParseIP(ifaceAddr.String())
		}
		if ip == nil {
			logger.Debugf("cannot parse %q as IP, ignoring", ifaceAddr)
			continue
		}
		ips = append(ips, ip)
		if ipNet != nil {
			ipNets = append(ipNets, ipNet)
		}
	}
	for _, addr := range allAddresses {
		found := false
		// Filter all known IPs
		for _, ip := range ips {
			if ip.String() == addr.Value {
				found = true
				break
			}
		}
		if !found {
			// Then check if it is in one of the CIDRs
			for _, ipNet := range ipNets {
				if ipNet.Contains(net.ParseIP(addr.Value)) {
					found = true
					break
				}
			}
		}
		if found {
			logger.Debugf("filtering %q address %s for machine", bridgeName, addr.String())
		} else {
			logger.Debugf("not filtering address %s for machine", addr)
			filtered = append(filtered, addr)
		}
	}
	logger.Debugf("addresses after filtering: %v", filtered)
	return filtered
}

// filterLXCAddresses tries to discover the default lxc bridge name
// and all of its addresses, then filters those addresses out of the
// given ones and returns the result. Any errors encountered during
// this process are logged, but not considered fatal. See LP bug
// #1416928.
func filterLXCAddresses(addresses []Address) []Address {
	file, err := os.Open(LXCNetDefaultConfig)
	if os.IsNotExist(err) {
		// No lxc-net config found, nothing to do.
		logger.Debugf("no lxc bridge addresses to filter for machine")
		return addresses
	} else if err != nil {
		// Just log it, as it's not fatal.
		logger.Errorf("cannot open %q: %v", LXCNetDefaultConfig, err)
		return addresses
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#"):
			// Skip comments.
		case strings.HasPrefix(line, "LXC_BRIDGE"):
			// Extract <name> from LXC_BRIDGE="<name>".
			parts := strings.Split(line, `"`)
			if len(parts) < 2 {
				logger.Debugf("ignoring invalid line '%s' in %q", line, LXCNetDefaultConfig)
				continue
			}
			bridgeName := strings.TrimSpace(parts[1])
			// Discover all addresses of bridgeName interface.
			addrs, err := InterfaceByNameAddrs(bridgeName)
			if err != nil {
				logger.Debugf("cannot get %q addresses: %v (ignoring)", bridgeName, err)
				continue
			}
			logger.Debugf("%q has addresses %v", bridgeName, addrs)
			return filterAddrs(bridgeName, addresses, addrs)
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Debugf("failed to read %q: %v (ignoring)", LXCNetDefaultConfig, err)
	}
	return addresses
}

// filterLXDAddresses removes addresses on the LXD bridge from the list to be
// considered.
func filterLXDAddresses(addresses []Address) []Address {
	// Should we be getting this from LXD instead?
	addrs, err := InterfaceByNameAddrs(DefaultLXDBridge)
	if err != nil {
		logger.Warningf("cannot get %q addresses: %v (ignoring)", DefaultLXDBridge, err)
		return addresses
	}
	logger.Debugf("%q has addresses %v", DefaultLXDBridge, addrs)
	return filterAddrs(DefaultLXDBridge, addresses, addrs)

}

// FilterBridgeAddresses removes addresses seen as a Bridge address (the IP
// address used only to connect to local containers), rather than a remote
// accessible address.
func FilterBridgeAddresses(addresses []Address) []Address {
	addresses = filterLXCAddresses(addresses)
	addresses = filterLXDAddresses(addresses)
	// TODO(jam): 2016-12-14 We should also be filtering Docker based
	// addresses, or any other 'local only' bridge addresses.
	return addresses
}
