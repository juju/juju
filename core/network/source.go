// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"io/ioutil"
	"net"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
)

// SysClassNetRoot is the full Linux SYSFS path containing
// information about each network interface on the system.
// TODO (manadart 2021-02-12): This remains in the main "source.go" module
// because there was previously only one ConfigSource implementation,
// which presumably did not work on Windows.
// When the NetlinkConfigSource was introduced for use on Linux,
// we retained the old universal config source for use on Windows.
// If there comes a time when we properly implement a Windows source,
// this should be relocated to the Linux module and an appropriate counterpart
// introduced for Windows.
const SysClassNetPath = "/sys/class/net"

// ConfigSourceAddr indirects addresses obtained
// from local machine network interfaces.
type ConfigSourceAddr interface {
	// IP returns the address in net.IP form.
	IP() net.IP

	// IPNet returns the subnet corresponding with the address
	// provided that it can be determined.
	IPNet() *net.IPNet

	// String returns the address in string form,
	// including the subnet mask if known.
	String() string
}

// ConfigSource defines the necessary calls to obtain
// the network configuration of a machine.
type ConfigSource interface {
	// SysClassNetPath returns the userspace SYSFS path used by this source.
	SysClassNetPath() string

	// Interfaces returns information about all
	// network interfaces on the machine.
	Interfaces() ([]net.Interface, error)

	// InterfaceAddresses returns information about all addresses
	// assigned to the network interface with the given name.
	InterfaceAddresses(name string) ([]ConfigSourceAddr, error)

	// DefaultRoute returns the gateway IP address and device name of the
	// default route on the machine. If there is no default route (known),
	// then zero values are returned.
	DefaultRoute() (net.IP, string, error)

	// OvsManagedBridges returns the names of network interfaces that
	// correspond to OVS-managed bridges.
	OvsManagedBridges() (set.Strings, error)
}

// ParseInterfaceType parses the DEVTYPE attribute from the Linux kernel
// userspace SYSFS location "<sysPath/<interfaceName>/uevent" and returns it as
// InterfaceType. SysClassNetPath should be passed as sysPath. Returns
// UnknownInterface if the type cannot be reliably determined for any reason.
// Example call: network.ParseInterfaceType(network.SysClassNetPath, "br-eth1")
// TODO (manadart 2021-02-12): As with SysClassNetPath above, specific
// implementations should be sought for this that are OS-dependent.
func ParseInterfaceType(sysPath, interfaceName string) InterfaceType {
	const deviceType = "DEVTYPE="
	location := filepath.Join(sysPath, interfaceName, "uevent")

	data, err := ioutil.ReadFile(location)
	if err != nil {
		logger.Debugf("ignoring error reading %q: %v", location, err)
		return UnknownInterface
	}

	devtype := ""
	lines := strings.Fields(string(data))
	for _, line := range lines {
		if !strings.HasPrefix(line, deviceType) {
			continue
		}

		devtype = strings.TrimPrefix(line, deviceType)
		switch devtype {
		case "bridge":
			return BridgeInterface
		case "vlan":
			return VLAN_8021QInterface
		case "bond":
			return BondInterface
		case "":
			// DEVTYPE is not present for some types, like Ethernet and loopback
			// interfaces, so if missing do not try to guess.
			break
		}
	}

	return UnknownInterface
}
