// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package network

import (
	"net"

	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
)

// netAddr implements ConfigSourceAddr based on an address in string form.
type netAddr struct {
	addr  string
	ip    net.IP
	ipNet *net.IPNet
}

// newNetAddr returns a new netAddr reference
// representing the input IP address string.
func newNetAddr(a string) (*netAddr, error) {
	res := &netAddr{
		addr: a,
	}

	ip, ipNet, err := net.ParseCIDR(a)
	if ipNet != nil {
		res.ipNet = ipNet
	}

	if ip == nil {
		ip = net.ParseIP(a)
	}

	if ip == nil {
		if err != nil {
			return nil, errors.Capture(err)
		}
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

// IsSecondary (ConfigSourceAddr) always returns false for
// addresses from the non-*nix config source.
// The Go stdlib does not surface the IFA_F_SECONDARY flag.
func (a *netAddr) IsSecondary() bool {
	return false
}

// String (ConfigSourceAddr) is a simple property accessor.
func (a *netAddr) String() string {
	return a.addr
}

// netNIC implements ConfigSourceNIC by wrapping a network interface
// reference from the standard library `net` package.
type netNIC struct {
	nic       *net.Interface
	parseType func(string) LinkLayerDeviceType
}

func newNetNIC(nic *net.Interface, parseType func(string) LinkLayerDeviceType) *netNIC {
	return &netNIC{
		nic:       nic,
		parseType: parseType,
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
func (n *netNIC) Type() LinkLayerDeviceType {
	nicType := n.parseType(n.Name())

	if nicType != UnknownDevice {
		return nicType
	}

	if n.nic.Flags&net.FlagLoopback > 0 {
		return LoopbackDevice
	}

	// See comment on super-method.
	// This is incorrect for veth, tuntap, macvtap et al.
	return EthernetDevice
}

// HardwareAddr returns the hardware address of the device.
func (n *netNIC) HardwareAddr() net.HardwareAddr {
	return n.nic.HardwareAddr
}

// Addresses returns all IP addresses associated with the device.
func (n *netNIC) Addresses() ([]ConfigSourceAddr, error) {
	addrs, err := n.nic.Addrs()
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]ConfigSourceAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr.String() != "" {
			a, err := newNetAddr(addr.String())
			if err != nil {
				return nil, errors.Capture(err)
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

type netPackageConfigSource struct {
	sysClassNetPath string
	interfaces      func() ([]net.Interface, error)
}

// Interfaces returns the network interfaces on the machine.
func (s *netPackageConfigSource) Interfaces() ([]ConfigSourceNIC, error) {
	nics, err := s.interfaces()
	if err != nil {
		return nil, errors.Capture(err)
	}

	parseType := func(name string) LinkLayerDeviceType { return ParseInterfaceType(s.sysClassNetPath, name) }
	result := make([]ConfigSourceNIC, len(nics))
	for i := range nics {
		// Close over the sysClassNetPath so that
		// the NIC needs to know nothing about it.
		result[i] = newNetNIC(&nics[i], parseType)
	}
	return result, nil
}

// OvsManagedBridges implements NetworkConfigSource.
func (*netPackageConfigSource) OvsManagedBridges() (set.Strings, error) {
	return OvsManagedBridges()
}

// DefaultRoute implements NetworkConfigSource.
func (*netPackageConfigSource) DefaultRoute() (net.IP, string, error) {
	return GetDefaultRoute()
}

// GetBridgePorts implements NetworkConfigSource.
func (s *netPackageConfigSource) GetBridgePorts(bridgeName string) []string {
	return GetBridgePorts(s.sysClassNetPath, bridgeName)
}

// DefaultConfigSource returns a NetworkConfigSource backed by
// the net package, to be used with GetObservedNetworkConfig().
func DefaultConfigSource() ConfigSource {
	return &netPackageConfigSource{
		sysClassNetPath: SysClassNetPath,
		interfaces:      net.Interfaces,
	}
}
