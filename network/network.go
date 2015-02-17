// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.network")

// Id of the default public juju network
const DefaultPublic = "juju-public"

// Id of the default private juju network
const DefaultPrivate = "juju-private"

// Id defines a provider-specific network id.
type Id string

// BasicInfo describes the bare minimum information for a network,
// which the provider knows about but juju might not yet.
type BasicInfo struct {
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
}

// Info describes a single network interface available on an instance.
// For providers that support networks, this will be available at
// StartInstance() time.
type Info struct {
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

	// ProviderId is a provider-specific network id.
	ProviderId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the raw OS-specific network device name (e.g.
	// "eth1", even for a VLAN eth1.42 virtual interface).
	InterfaceName string

	// Disabled is true when the interface needs to be disabled on the
	// machine, e.g. not to configure it.
	Disabled bool
}

// ActualInterfaceName returns raw interface name for raw interface (e.g. "eth0") and
// virtual interface name for virtual interface (e.g. "eth0.42")
func (i *Info) ActualInterfaceName() string {
	if i.VLANTag > 0 {
		return fmt.Sprintf("%s.%d", i.InterfaceName, i.VLANTag)
	}
	return i.InterfaceName
}

// IsVirtual returns true when the interface is a virtual device, as
// opposed to a physical device (e.g. a VLAN or a network alias)
func (i *Info) IsVirtual() bool {
	return i.VLANTag > 0
}

// IsVLAN returns true when the interface is a VLAN interface.
func (i *Info) IsVLAN() bool {
	return i.VLANTag > 0
}

// PreferIPv6Getter will be implemented by both the environment and agent
// config.
type PreferIPv6Getter interface {
	PreferIPv6() bool
}

// InitializeFromConfig needs to be called once after the environment
// or agent configuration is available to configure networking
// settings.
func InitializeFromConfig(config PreferIPv6Getter) {
	globalPreferIPv6 = config.PreferIPv6()
	logger.Infof("setting prefer-ipv6 to %v", globalPreferIPv6)
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
	}
	if err := scanner.Err(); err != nil {
		logger.Warningf("failed to read %q: %v (ignoring)", LXCNetDefaultConfig, err)
	}
	return addresses
}
