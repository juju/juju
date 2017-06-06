// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"net"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
)

var _ api.IPAddrResolver = IPAddrResolverMap(nil)

// IPAddrResolverMap implements IPAddrResolver by looking up the
// addresses in the map, which maps host names to IP addresses. The
// strings in the value slices should be valid IP addresses.
type IPAddrResolverMap map[string][]string

func (r IPAddrResolverMap) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	ipStrs := r[host]
	if len(ipStrs) == 0 {
		return nil, errors.Errorf("mock resolver cannot resolve %q", host)
	}
	ipAddrs := make([]net.IPAddr, len(ipStrs))
	for i, ipStr := range ipStrs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			panic("invalid IP address: " + ipStr)
		}
		ipAddrs[i] = net.IPAddr{
			IP: ip,
		}
	}
	return ipAddrs, nil
}
