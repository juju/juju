// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/juju/juju/internal/errors"
)

// CIDRAddressType returns back an AddressType to indicate whether the supplied
// CIDR corresponds to an IPV4 or IPV6 range. An error will be returned if a
// non-valid CIDR is provided.
func CIDRAddressType(cidrStr string) (AddressType, error) {
	cidr, err := ParseCIDR(cidrStr)
	if err != nil {
		return "", err
	}

	if cidr.Addr().Is4() {
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

// CIDR wraps a [netip.Prefix] instance and provides a
// method to return the start and end address range.
type CIDR struct {
	netip.Prefix
}

// ParseCIDR parses the specified cidr string.
func ParseCIDR(cidr string) (*CIDR, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, errors.Errorf("parsing CIDR %q: %w", cidr, err)
	}
	if prefix.Masked().Addr() != prefix.Addr() {
		return nil, errors.Errorf("CIDR %q is not valid", cidr)
	}
	return &CIDR{Prefix: prefix}, nil
}

// AddressRange returns the start and end address for the cidr.
func (c CIDR) AddressRange() (start, end IPAddress) {
	start = IPAddress{c.Masked().Addr()}

	m := net.CIDRMask(c.Bits(), c.Addr().BitLen())
	endSlice := c.Addr().AsSlice()
	for i := 0; i < len(endSlice); i++ {
		endSlice[i] = endSlice[i] | ^m[i]
	}
	endAddr, _ := netip.AddrFromSlice(endSlice)
	end = IPAddress{endAddr}
	return start, end
}
