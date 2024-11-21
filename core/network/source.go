// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
)

// SysClassNetPath is the full Linux SYSFS path containing
// information about each network interface on the system.
// TODO (manadart 2021-02-12): This remains in the main "source.go" module
// because there was previously only one ConfigSource implementation,
// which presumably did not work on Windows.
// When the netlinkConfigSource was introduced for use on Linux,
// we retained the old universal config source for use on Windows.
// If there comes a time when we properly implement a Windows source,
// this should be relocated to the Linux module and an appropriate counterpart
// introduced for Windows.
const SysClassNetPath = "/sys/class/net"

// ConfigSourceNIC describes a network interface detected on the local machine
// by an implementation of ConfigSource.
type ConfigSourceNIC interface {
	// Name returns the name of the network interface; E.g. "eth0".
	Name() string

	// Type returns the type of the interface - Ethernet, VLAN, Loopback etc.
	// TODO (manadart 2021-03-03): We do not recognise device types such as
	// veth, tuntap, macvtap et al. Our parsing falls back to ethernet for such
	// devices, which we should change in order to have a better informed
	// networking model.
	Type() LinkLayerDeviceType

	// Index returns the index of the interface.
	Index() int

	// HardwareAddr returns the hardware address of the interface.
	// It is the MAC address for ethernet devices.
	HardwareAddr() net.HardwareAddr

	// Addresses returns IP addresses associated with the network interface.
	Addresses() ([]ConfigSourceAddr, error)

	// MTU returns the maximum transmission unit for the interface.
	MTU() int

	// IsUp returns true if the interface is in the "up" state.
	IsUp() bool
}

// ConfigSourceAddr describes addresses detected on a network interface
// represented by an implementation of ConfigSourceAddr.
type ConfigSourceAddr interface {
	// IP returns the address in net.IP form.
	IP() net.IP

	// IPNet returns the subnet corresponding with the address
	// provided that it can be determined.
	IPNet() *net.IPNet

	// IsSecondary returns true if this address can be determined not to be
	// the primary address of its NIC.
	// Such addresses are added by HA setups like Corosync+Pacemaker.
	IsSecondary() bool

	// String returns the address in string form,
	// including the subnet mask if known.
	String() string
}

// ConfigSource defines the necessary calls to obtain
// the network configuration of a machine.
type ConfigSource interface {
	// Interfaces returns information about all
	// network interfaces on the machine.
	Interfaces() ([]ConfigSourceNIC, error)

	// DefaultRoute returns the gateway IP address and device name of the
	// default route on the machine. If there is no default route (known),
	// then zero values are returned.
	DefaultRoute() (net.IP, string, error)

	// OvsManagedBridges returns the names of network interfaces that
	// correspond to OVS-managed bridges.
	OvsManagedBridges() (set.Strings, error)

	// GetBridgePorts returns the names of network interfaces that are ports ot
	// the bridge with the input device name.
	GetBridgePorts(string) []string
}

// ParseInterfaceType parses the DEVTYPE attribute from the Linux kernel
// userspace SYSFS location "<sysPath/<interfaceName>/uevent" and returns it as
// InterfaceType. SysClassNetPath should be passed as sysPath. Returns
// UnknownInterface if the type cannot be reliably determined for any reason.
// Example call: network.ParseInterfaceType(network.SysClassNetPath, "br-eth1")
// TODO (manadart 2021-02-12): As with SysClassNetPath above, specific
// implementations should be sought for this that are OS-dependent.
func ParseInterfaceType(sysPath, interfaceName string) LinkLayerDeviceType {
	const deviceType = "DEVTYPE="
	location := filepath.Join(sysPath, interfaceName, "uevent")

	data, err := os.ReadFile(location)
	if err != nil {
		logger.Debugf(context.TODO(), "ignoring error reading %q: %v", location, err)
		return UnknownDevice
	}

	var devType string
	lines := strings.Fields(string(data))
	for _, line := range lines {
		if !strings.HasPrefix(line, deviceType) {
			continue
		}

		devType = strings.TrimPrefix(line, deviceType)
		switch devType {
		case "bridge":
			return BridgeDevice
		case "vlan":
			return VLAN8021QDevice
		case "bond":
			return BondDevice
		case "":
			// DEVTYPE is not present for some types, like Ethernet and loopback
			// interfaces, so if missing do not try to guess.
			break
		}
	}

	return UnknownDevice
}

// GetBridgePorts extracts and returns the names of all interfaces configured as
// ports of the given bridgeName from the Linux kernel userspace SYSFS location
// "<sysPath/<bridgeName>/brif/*". SysClassNetPath should be passed as sysPath.
// Returns an empty result if the ports cannot be determined reliably for any
// reason, or if there are no configured ports for the bridge.
// Example call: network.GetBridgePorts(network.SysClassNetPath, "br-eth1")
// TODO (manadart 2021-02-12): As with SysClassNetPath above, specific
// implementations should be sought for this that are OS-dependent.
func GetBridgePorts(sysPath, bridgeName string) []string {
	portsGlobPath := filepath.Join(sysPath, bridgeName, "brif", "*")
	// Glob ignores I/O errors and can only return ErrBadPattern, which we treat
	// as no results, but for debugging we're still logging the error.
	paths, err := filepath.Glob(portsGlobPath)
	if err != nil {
		logger.Debugf(context.TODO(), "ignoring error traversing path %q: %v", portsGlobPath, err)
	}

	if len(paths) == 0 {
		return nil
	}

	// We need to convert full paths like /sys/class/net/br-eth0/brif/eth0 to
	// just names.
	names := make([]string, len(paths))
	for i := range paths {
		names[i] = filepath.Base(paths[i])
	}
	return names
}
