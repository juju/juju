// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"net"
	"strings"

	"github.com/juju/juju/core/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
)

// markDefaultRouteSource returns the space addresses for the input unit
// addresses, with IsDefaultRouteSource set on the primary IPv4 address(es)
// of the default route device whose subnet contains the gateway. If none
// contains it, all primary IPv4 addresses on that device are marked.
// The input order is preserved.
func markDefaultRouteSource(
	addrs []networkinternal.UnitDeviceAddress,
) network.SpaceAddresses {
	result := make(network.SpaceAddresses, len(addrs))

	// Indexes of the candidate addresses, grouped by device.
	candidates := make(map[string][]int)
	for i, addr := range addrs {
		result[i] = addr.SpaceAddress
		if isDefaultRouteCandidate(addr) {
			candidates[addr.DeviceUUID] = append(candidates[addr.DeviceUUID], i)
		}
	}

	for _, indexes := range candidates {
		// The gateway is a property of the device, so is the same for all of
		// its addresses.
		gateway := parseGateway(addrs[indexes[0]].GatewayAddress)

		var matched []int
		if gateway != nil {
			for _, i := range indexes {
				if cidrContains(addrs[i].CIDR, gateway) {
					matched = append(matched, i)
				}
			}
		}
		if len(matched) == 0 {
			matched = indexes
		}

		for _, i := range matched {
			result[i].IsDefaultRouteSource = true
		}
	}

	return result
}

// isDefaultRouteCandidate returns true if the address can be the source of the
// IPv4 default route. That is, it is a primary IPv4 address on a device that
// carries the default route.
func isDefaultRouteCandidate(addr networkinternal.UnitDeviceAddress) bool {
	return addr.IsDefaultGatewayDevice &&
		!addr.IsSecondary &&
		addr.Type == network.IPv4Address
}

// parseGateway returns the IPv4 gateway address, or nil if the input is empty,
// unparsable or not an IPv4 address. Discovery can store junk in the gateway
// address, such as "<nil>" when the default route has no next hop.
func parseGateway(gateway string) net.IP {
	ip := net.ParseIP(strings.TrimSpace(gateway))
	if ip == nil {
		return nil
	}
	return ip.To4()
}

// cidrContains returns true if the CIDR is valid and contains the IP address.
func cidrContains(cidr string, ip net.IP) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return ipNet.Contains(ip)
}
