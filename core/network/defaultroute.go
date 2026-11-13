// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"
	"strings"
)

// MarkDefaultRouteSource returns the space addresses for the input device
// addresses, in the same order. For each device that carries the IPv4
// default route, it sets IsDefaultRouteSource on the primary IPv4 addresses
// whose CIDR contains the device's gateway. If the gateway is missing or
// not a valid IPv4 address, or no CIDR contains it, all primary IPv4
// addresses on that device are marked.
func MarkDefaultRouteSource(addrs []DeviceAddress) SpaceAddresses {
	result := make(SpaceAddresses, len(addrs))

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
func isDefaultRouteCandidate(addr DeviceAddress) bool {
	return addr.IsDefaultGatewayDevice &&
		!addr.IsSecondary &&
		addr.Type == IPv4Address
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
