// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/juju/errors"
)

// netAddr implements ConfigSourceAddr based on an address in string form.
type netAddr struct {
	addr  string
	ip    net.IP
	ipNet *net.IPNet
}

// NewNetAddr returns a new netAddr reference representing the input IP address
// string. No error is returned; instead the validity of input is indicated by
// the return having a populated ip member.
// TODO (manadart 2021-02-15): This method is exported on account of testing in
// the api/common package where this logic used to reside and where the actual
// detection and conversion to params is invoked.
// The detection should also be relocated here to core/network in order that
// the export is no longer required.
func NewNetAddr(a string) *netAddr {
	res := &netAddr{
		addr: a,
	}

	ip, ipNet, _ := net.ParseCIDR(a)
	if ipNet != nil {
		res.ipNet = ipNet
	}

	if ip == nil {
		ip = net.ParseIP(a)
	}
	res.ip = ip

	return res
}

// IP (ConfigSourceAddr) is a simple property accessor.
func (a *netAddr) IP() net.IP {
	return a.ip
}

// IPNet (ConfigSourceAddr) is a simple property accessor.
func (a *netAddr) IPNet() *net.IPNet {
	return a.ipNet
}

// String (ConfigSourceAddr) is a simple property accessor.
func (a *netAddr) String() string {
	return a.addr
}

type netPackageConfigSource struct {
	interfaces      func() ([]net.Interface, error)
	interfaceByName func(name string) (*net.Interface, error)
}

// SysClassNetPath returns the system path containing information
// about a machine's network interfaces.
func (n *netPackageConfigSource) SysClassNetPath() string {
	return SysClassNetPath
}

// Interfaces returns the network interfaces on the machine.
func (n *netPackageConfigSource) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddresses the addresses associated with the network
// interface identified by the input name.
func (n *netPackageConfigSource) InterfaceAddresses(name string) ([]ConfigSourceAddr, error) {
	nic, err := n.interfaceByName(name)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving network interface %q", name)
	}

	addrs, err := nic.Addrs()
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving addresses for interface %q", name)
	}

	result := make([]ConfigSourceAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr.String() != "" {
			result = append(result, NewNetAddr(addr.String()))
		}
	}
	return result, nil
}

// DefaultRoute implements NetworkConfigSource.
func (n *netPackageConfigSource) DefaultRoute() (net.IP, string, error) {
	return GetDefaultRoute()
}

// DefaultNetworkConfigSource returns a NetworkConfigSource backed by the net
// package, to be used with GetObservedNetworkConfig().
func DefaultNetworkConfigSource() ConfigSource {
	return &netPackageConfigSource{
		interfaces:      net.Interfaces,
		interfaceByName: net.InterfaceByName,
	}
}
