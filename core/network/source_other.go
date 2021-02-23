// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// netNIC implements ConfigSourceNIC by wrapping a network interface
// reference from the standard library `net` package.
type netNIC struct {
	sysClassNetPath string
	nic             *net.Interface
}

func NewNetNIC(sysClassNetPath string, nic *net.Interface) *netNIC {
	return &netNIC{
		sysClassNetPath: sysClassNetPath,
		nic:             nic,
	}
}

// Name returns the name of the device.
func (n *netNIC) Name() string {
	return n.nic.Name
}

// Index returns the index of the device.
func (n *netNIC) Index() int {
	return n.nic.Index
}

// Type returns the interface type of the device.
func (n *netNIC) Type() InterfaceType {
	nicType := ParseInterfaceType(n.sysClassNetPath, n.Name())

	if nicType != UnknownInterface {
		return nicType
	}

	if n.nic.Flags&net.FlagLoopback > 0 {
		return LoopbackInterface
	}

	return EthernetInterface
}

// HardwareAddr returns the hardware address of the device.
func (n *netNIC) HardwareAddr() net.HardwareAddr {
	return n.nic.HardwareAddr
}

// Addresses returns all IP addresses associated with the device.
func (n *netNIC) Addresses() ([]ConfigSourceAddr, error) {
	addrs, err := n.nic.Addrs()
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving addresses for interface %q", n.nic.Name)
	}

	result := make([]ConfigSourceAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr.String() != "" {
			a, err := NewNetAddr(addr.String())
			if err != nil {
				return nil, errors.Trace(err)
			}

			result = append(result, a)
		}
	}
	return result, nil
}

// MTU returns the maximum transmission unit for the device.
func (n *netNIC) MTU() int {
	return n.nic.MTU
}

// IsUp returns true if the interface is in the "up" state.
func (n *netNIC) IsUp() bool {
	return n.nic.Flags&net.FlagUp > 0
}

// netAddr implements ConfigSourceAddr based on an address in string form.
type netAddr struct {
	addr  string
	ip    net.IP
	ipNet *net.IPNet
}

// NewNetAddr returns a new netAddr reference
// representing the input IP address string.
// TODO (manadart 2021-02-15): This method is exported on account of testing in
// the api/common package where this logic used to reside and where the actual
// detection and conversion to params is invoked.
// The detection should also be relocated here to core/network in order that
// the export is no longer required.
func NewNetAddr(a string) (*netAddr, error) {
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

	if ip == nil {
		return nil, errors.Errorf("unable to parse IP address %q", a)
	}

	res.ip = ip
	return res, nil
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
	interfaces func() ([]net.Interface, error)
}

// SysClassNetPath returns the system path containing information
// about a machine's network interfaces.
func (n *netPackageConfigSource) SysClassNetPath() string {
	return SysClassNetPath
}

// Interfaces returns the network interfaces on the machine.
func (n *netPackageConfigSource) Interfaces() ([]ConfigSourceNIC, error) {
	nics, err := n.interfaces()
	if err != nil {
		return nil, errors.Annotate(err, "detecting network interfaces")
	}

	result := make([]ConfigSourceNIC, len(nics))
	for i := range nics {
		result[i] = NewNetNIC(n.SysClassNetPath(), &nics[i])
	}
	return result, nil
}

func (n *netPackageConfigSource) OvsManagedBridges() (set.Strings, error) {
	return OvsManagedBridges()
}

// DefaultRoute implements NetworkConfigSource.
func (n *netPackageConfigSource) DefaultRoute() (net.IP, string, error) {
	return GetDefaultRoute()
}

// DefaultNetworkConfigSource returns a NetworkConfigSource backed by the net
// package, to be used with GetObservedNetworkConfig().
func DefaultNetworkConfigSource() ConfigSource {
	return &netPackageConfigSource{
		interfaces: net.Interfaces,
	}
}
