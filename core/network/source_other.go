// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/juju/errors"
)

type netPackageConfigSource struct{}

// SysClassNetPath implements NetworkConfigSource.
func (n *netPackageConfigSource) SysClassNetPath() string {
	return SysClassNetPath
}

// Interfaces implements NetworkConfigSource.
func (n *netPackageConfigSource) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddresses implements NetworkConfigSource.
func (n *netPackageConfigSource) InterfaceAddresses(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return iface.Addrs()
}

// DefaultRoute implements NetworkConfigSource.
func (n *netPackageConfigSource) DefaultRoute() (net.IP, string, error) {
	return GetDefaultRoute()
}

// DefaultNetworkConfigSource returns a NetworkConfigSource backed by the net
// package, to be used with GetObservedNetworkConfig().
func DefaultNetworkConfigSource() ConfigSource {
	return &netPackageConfigSource{}
}
