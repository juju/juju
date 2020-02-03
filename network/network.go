// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	corenetwork "github.com/juju/juju/core/network"
)

var logger = loggo.GetLogger("juju.network")

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

// UnknownId can be used whenever an Id is needed but not known.
const UnknownId = ""

// DefaultLXCBridge is the bridge that gets used for LXC containers
const DefaultLXCBridge = "lxcbr0"

// DefaultLXDBridge is the bridge that gets used for LXD containers
const DefaultLXDBridge = "lxdbr0"

// DefaultKVMBridge is the bridge that is set up by installing libvirt-bin
// Note: we don't import this from 'container' to avoid import loops
const DefaultKVMBridge = "virbr0"

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
	ProviderId corenetwork.Id

	// ProviderSubnetId is the provider-specific id for the associated
	// subnet.
	ProviderSubnetId corenetwork.Id

	// ProviderNetworkId is the provider-specific id for the
	// associated network.
	ProviderNetworkId corenetwork.Id

	// ProviderSpaceId is the provider-specific id for the associated space, if
	// known and supported.
	ProviderSpaceId corenetwork.Id

	// ProviderVLANId is the provider-specific id of the VLAN for this
	// interface.
	ProviderVLANId corenetwork.Id

	// ProviderAddressId is the provider-specific id of the assigned address.
	ProviderAddressId corenetwork.Id

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
	Address corenetwork.ProviderAddress

	// DNSServers contains an optional list of IP addresses and/or
	// hostnames to configure as DNS servers for this network
	// interface.
	DNSServers corenetwork.ProviderAddresses

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
	GatewayAddress corenetwork.ProviderAddress

	// Routes defines a list of routes that should be added when this interface
	// is brought up, and removed when this interface is stopped.
	Routes []Route

	// IsDefaultGateway is set if this device is a default gw on a machine.
	IsDefaultGateway bool
}

// Route defines a single route to a subnet via a defined gateway.
type Route struct {
	// DestinationCIDR is the subnet that we want a controlled route to.
	DestinationCIDR string
	// GatewayIP is the IP (v4 or v6) that should be used for traffic that is
	// bound for DestinationCIDR
	GatewayIP string
	// Metric is the weight to apply to this route.
	Metric int
}

// InterfaceAddress represents a single address attached to the interface.
type InterfaceAddress struct {
	Address string
	CIDR    string
}

// NetworkInfo describes one interface with assigned IP addresses, it's a mirror of params.NetworkInfo.
type NetworkInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// InterfaceName is the OS-specific interface name, eg. "eth0" or "eno1.412"
	InterfaceName string

	// Addresses contains a list of addresses configured on the interface.
	Addresses []InterfaceAddress
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
	ProviderId corenetwork.Id

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

	// BridgeName is the name of the bridge that we want created.
	BridgeName string

	// MACAddress is the MAC address of the device to be bridged
	MACAddress string
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

type ipNetAndName struct {
	ipnet *net.IPNet
	name  string
}

func addrMapToIPNetAndName(bridgeToAddrs map[string][]net.Addr) []ipNetAndName {
	ipNets := make([]ipNetAndName, 0, len(bridgeToAddrs))
	for bridgeName, addrList := range bridgeToAddrs {
		for _, ifaceAddr := range addrList {
			ip, ipNet, err := net.ParseCIDR(ifaceAddr.String())
			if err != nil {
				// Not a valid CIDR, check as an IP
				ip = net.ParseIP(ifaceAddr.String())
			}
			if ip == nil {
				logger.Debugf("cannot parse %q as IP, ignoring", ifaceAddr)
				continue
			}
			if ipNet == nil {
				// convert the IP into an IPNet
				if ip.To4() != nil {
					_, ipNet, err = net.ParseCIDR(ip.String() + "/32")
					if err != nil {
						logger.Debugf("error creating a /32 CIDR for %q", ifaceAddr)
					}
				} else if ip.To16() != nil {
					_, ipNet, err = net.ParseCIDR(ip.String() + "/128")
					if err != nil {
						logger.Debugf("error creating a /128 CIDR for %q", ifaceAddr)
					}
				} else {
					logger.Debugf("failed to convert %q to a v4 or v6 address, ignoring", ifaceAddr)
				}
			}
			ipNets = append(ipNets, ipNetAndName{ipnet: ipNet, name: bridgeName})
		}
	}
	return ipNets
}

// filterAddrs looks at all of the addresses in allAddresses and removes ones
// that line up with removeAddresses. Note that net.Addr may be just an IP or
// may be a CIDR.  removeAddresses should be a map of 'bridge name' to list of
// addresses, so that we can report why the address was filtered.
func filterAddrs(
	allAddresses []corenetwork.ProviderAddress, removeAddresses map[string][]net.Addr,
) []corenetwork.ProviderAddress {
	filtered := make([]corenetwork.ProviderAddress, 0, len(allAddresses))
	// Convert all
	ipNets := addrMapToIPNetAndName(removeAddresses)
	for _, addr := range allAddresses {
		bridgeName := ""
		// Then check if it is in one of the CIDRs
		ip := net.ParseIP(addr.Value)
		if ip == nil {
			logger.Debugf("not filtering invalid IP: %q", addr.Value)
		} else {
			for _, ipNetName := range ipNets {
				if ipNetName.ipnet.Contains(ip) {
					bridgeName = ipNetName.name
					break
				}
			}
		}
		if bridgeName == "" {
			logger.Debugf("including address %v for machine", addr)
			filtered = append(filtered, addr)
		} else {
			logger.Debugf("filtering %q address %s for machine", bridgeName, addr.String())
		}
	}
	return filtered
}

// gatherLXCAddresses tries to discover the default lxc bridge name
// and all of its addresses. See LP bug #1416928.
func gatherLXCAddresses(toRemove map[string][]net.Addr) {
	file, err := os.Open(LXCNetDefaultConfig)
	if os.IsNotExist(err) {
		// No lxc-net config found, nothing to do.
		logger.Debugf("no lxc bridge addresses to filter for machine")
		return
	} else if err != nil {
		// Just log it, as it's not fatal.
		logger.Errorf("cannot open %q: %v", LXCNetDefaultConfig, err)
		return
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
			gatherBridgeAddresses(bridgeName, toRemove)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Debugf("failed to read %q: %v (ignoring)", LXCNetDefaultConfig, err)
	}
	return
}

func gatherBridgeAddresses(bridgeName string, toRemove map[string][]net.Addr) {
	addrs, err := InterfaceByNameAddrs(bridgeName)
	if err != nil {
		logger.Debugf("cannot get %q addresses: %v (ignoring)", bridgeName, err)
		return
	}
	logger.Debugf("%q has addresses %v", bridgeName, addrs)
	toRemove[bridgeName] = addrs
	return

}

// FilterBridgeAddresses removes addresses seen as a Bridge address
// (the IP address used only to connect to local containers),
// rather than a remote accessible address.
func FilterBridgeAddresses(addresses []corenetwork.ProviderAddress) corenetwork.ProviderAddresses {
	addressesToRemove := make(map[string][]net.Addr)
	gatherLXCAddresses(addressesToRemove)
	gatherBridgeAddresses(DefaultLXDBridge, addressesToRemove)
	gatherBridgeAddresses(DefaultKVMBridge, addressesToRemove)
	filtered := filterAddrs(addresses, addressesToRemove)
	logger.Debugf("addresses after filtering: %v", filtered)
	return filtered
}

// QuoteSpaces takes a slice of space names, and returns a nicely formatted
// form so they show up legible in log messages, etc.
func QuoteSpaces(vals []string) string {
	out := []string{}
	if len(vals) == 0 {
		return "<none>"
	}
	for _, space := range vals {
		out = append(out, fmt.Sprintf("%q", space))
	}
	return strings.Join(out, ", ")
}

// QuoteSpaceSet is the same as QuoteSpaces, but ensures that a set.Strings
// gets sorted values output.
func QuoteSpaceSet(vals set.Strings) string {
	return QuoteSpaces(vals.SortedValues())
}

// firstLastAddresses returns the first and last addresses of the subnet.
func firstLastAddresses(subnet *net.IPNet) (net.IP, net.IP) {
	firstIP := subnet.IP
	lastIP := make([]byte, len(firstIP))
	copy(lastIP, firstIP)

	for i, b := range lastIP {
		lastIP[i] = b ^ (^subnet.Mask[i])
	}
	return firstIP, lastIP
}

func cidrContains(cidr *net.IPNet, subnet *net.IPNet) bool {
	first, last := firstLastAddresses(subnet)
	return cidr.Contains(first) && cidr.Contains(last)
}

// SubnetInAnyRange returns true if the subnet's address range is fully
// contained in any of the specified subnet blocks.
func SubnetInAnyRange(cidrs []*net.IPNet, subnet *net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidrContains(cidr, subnet) {
			return true
		}
	}
	return false
}

// Export for testing
var ResolverFunc = net.ResolveIPAddr

// FormatAsCIDR converts the specified IP addresses to a slice of CIDRs. It
// attempts to resolve any address represented as hostnames before formatting.
func FormatAsCIDR(addresses []string) ([]string, error) {
	result := make([]string, len(addresses))
	for i, a := range addresses {
		cidr := a
		// If address is not already a cidr, add a /32 (ipv4) or /128 (ipv6).
		if _, _, err := net.ParseCIDR(a); err != nil {
			address, err := ResolverFunc("ip", a)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if address.IP.To4() != nil {
				cidr = address.String() + "/32"
			} else {
				cidr = address.String() + "/128"
			}
		}
		result[i] = cidr
	}
	return result, nil
}
