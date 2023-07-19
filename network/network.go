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

// UnknownId can be used whenever an Id is needed but not known.
const UnknownId = ""

// DefaultLXCBridge is the bridge that gets used for LXC containers
const DefaultLXCBridge = "lxcbr0"

// DefaultLXDBridge is the bridge that gets used for LXD containers
const DefaultLXDBridge = "lxdbr0"

// DefaultKVMBridge is the bridge that is set up by installing libvirt-bin
// Note: we don't import this from 'container' to avoid import loops
const DefaultKVMBridge = "virbr0"

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

// AddressesForInterfaceName returns the addresses in string form for the
// given interface name. It's exported to facilitate cross-package testing.
var AddressesForInterfaceName = func(name string) ([]string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]string, len(addrs))
	for i, addr := range addrs {
		res[i] = addr.String()
	}
	return res, nil
}

type ipNetAndName struct {
	ipnet *net.IPNet
	name  string
}

func addrMapToIPNetAndName(bridgeToAddrs map[string][]string) []ipNetAndName {
	ipNets := make([]ipNetAndName, 0, len(bridgeToAddrs))
	for bridgeName, addrList := range bridgeToAddrs {
		for _, addr := range addrList {
			ip, ipNet, err := net.ParseCIDR(addr)
			if err != nil {
				// Not a valid CIDR, check as an IP
				ip = net.ParseIP(addr)
			}
			if ip == nil {
				logger.Debugf("cannot parse %q as IP, ignoring", addr)
				continue
			}
			if ipNet == nil {
				// convert the IP into an IPNet
				if ip.To4() != nil {
					_, ipNet, err = net.ParseCIDR(ip.String() + "/32")
					if err != nil {
						logger.Debugf("error creating a /32 CIDR for %q", addr)
					}
				} else if ip.To16() != nil {
					_, ipNet, err = net.ParseCIDR(ip.String() + "/128")
					if err != nil {
						logger.Debugf("error creating a /128 CIDR for %q", addr)
					}
				} else {
					logger.Debugf("failed to convert %q to a v4 or v6 address, ignoring", addr)
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
	allAddresses []corenetwork.ProviderAddress, removeAddresses map[string][]string,
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
func gatherLXCAddresses(toRemove map[string][]string) {
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
}

func gatherBridgeAddresses(bridgeName string, toRemove map[string][]string) {
	addrs, err := AddressesForInterfaceName(bridgeName)
	if err != nil {
		logger.Debugf("cannot get %q addresses: %v (ignoring)", bridgeName, err)
		return
	}
	logger.Debugf("%q has addresses %v", bridgeName, addrs)
	toRemove[bridgeName] = addrs
}

// FilterBridgeAddresses removes addresses seen as a Bridge address
// (the IP address used only to connect to local containers),
// rather than a remote accessible address.
// This includes addresses used by the local Fan network.
func FilterBridgeAddresses(addresses corenetwork.ProviderAddresses) corenetwork.ProviderAddresses {
	addressesToRemove := make(map[string][]string)
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
