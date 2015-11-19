// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.network")

// TODO(dimitern): Remove this once we use spaces as per the model.
const (
	// Id of the default public juju network
	DefaultPublic = "juju-public"

	// Id of the default private juju network
	DefaultPrivate = "juju-private"

	// Provider Id for the default network
	DefaultProviderId = "juju-unknown"
)

// DefaultSpace is the name used for the default space for an environment.
// TODO(dimitern): Make this configurable per environment.
const DefaultSpace = "default"

// noAddress represents an error when an address is requested but not available.
type noAddress struct {
	errors.Err
}

// NoAddressf returns an error which satisfies IsNoAddress().
func NoAddressf(format string, args ...interface{}) error {
	newErr := errors.NewErr(format+" no address", args...)
	newErr.SetLocation(1)
	return &noAddress{newErr}
}

// IsNoAddress reports whether err was created with NoAddressf().
func IsNoAddress(err error) bool {
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

	// AllocatableIPLow and AllocatableIPHigh describe the allocatable
	// portion of the subnet. The provider will only permit allocation
	// between these limits. If they are empty then none of the subnet is
	// allocatable.
	AllocatableIPLow  net.IP
	AllocatableIPHigh net.IP

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName holds the juju network space associated with this
	// subnet. Can be empty if not supported.
	SpaceName string
}

type SpaceInfo struct {
	Name  string
	CIDRs []string
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
	ConfigUnknown InterfaceConfigType = ""
	ConfigDHCP    InterfaceConfigType = "dhcp"
	ConfigStatic  InterfaceConfigType = "static"
	ConfigManual  InterfaceConfigType = "manual"
	// add others when needed
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

	// NetworkName is juju-internal name of the network.
	NetworkName string

	// ProviderId is a provider-specific NIC id.
	ProviderId Id

	// ProviderSubnetId is the provider-specific id for the associated
	// subnet.
	ProviderSubnetId Id

	// AvailabilityZones describes the availability zones the associated
	// subnet is in.
	AvailabilityZones []string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

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

	// DNSSearch contains the default DNS domain to use for
	// non-FQDN lookups.
	DNSSearch string

	// Gateway address, if set, defines the default gateway to
	// configure for this network interface. For containers this
	// usually is (one of) the host address(es).
	GatewayAddress Address

	// ExtraConfig can contain any valid setting and its value allowed
	// inside an "iface" section of a interfaces(5) config file, e.g.
	// "up", "down", "mtu", etc.
	ExtraConfig map[string]string
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

var preferIPv6 uint32

// PreferIPV6 returns true if this process prefers IPv6.
func PreferIPv6() bool { return atomic.LoadUint32(&preferIPv6) > 0 }

// SetPreferIPv6 determines whether IPv6 addresses will be preferred when
// selecting a public or internal addresses, using the Select*() methods.
// SetPreferIPV6 needs to be called to set this flag globally at the
// earliest time possible (e.g. at bootstrap, agent startup, before any
// CLI command).
func SetPreferIPv6(prefer bool) {
	var b uint32
	if prefer {
		b = 1
	}
	atomic.StoreUint32(&preferIPv6, b)
	logger.Infof("setting prefer-ipv6 to %v", prefer)
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

// FilterLXCAddresses tries to discover the default lxc bridge name
// and all of its addresses, then filters those addresses out of the
// given ones and returns the result. Any errors encountered during
// this process are logged, but not considered fatal. See LP bug
// #1416928.
func FilterLXCAddresses(addresses []Address) []Address {
	file, err := os.Open(LXCNetDefaultConfig)
	if os.IsNotExist(err) {
		// No lxc-net config found, nothing to do.
		logger.Debugf("no lxc bridge addresses to filter for machine")
		return addresses
	} else if err != nil {
		// Just log it, as it's not fatal.
		logger.Warningf("cannot open %q: %v", LXCNetDefaultConfig, err)
		return addresses
	}
	defer file.Close()

	filterAddrs := func(bridgeName string, addrs []net.Addr) []Address {
		// Filter out any bridge addresses.
		filtered := make([]Address, 0, len(addresses))
		for _, addr := range addresses {
			found := false
			for _, ifaceAddr := range addrs {
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
				// Filter by CIDR if known or single IP otherwise.
				if ipNet != nil && ipNet.Contains(net.ParseIP(addr.Value)) ||
					ip.String() == addr.Value {
					found = true
					logger.Debugf("filtering %q address %s for machine", bridgeName, ifaceAddr.String())
				}
			}
			if !found {
				logger.Debugf("not filtering address %s for machine", addr)
				filtered = append(filtered, addr)
			}
		}
		logger.Debugf("addresses after filtering: %v", filtered)
		return filtered
	}

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
				logger.Warningf("cannot get %q addresses: %v (ignoring)", bridgeName, err)
				continue
			}
			logger.Debugf("%q has addresses %v", bridgeName, addrs)
			return filterAddrs(bridgeName, addrs)
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warningf("failed to read %q: %v (ignoring)", LXCNetDefaultConfig, err)
	}
	return addresses
}
