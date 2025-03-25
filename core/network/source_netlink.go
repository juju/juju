// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package network

import (
	"net"

	"github.com/juju/collections/set"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/juju/juju/internal/errors"
)

// netlinkAddr implements ConfigSourceAddr based on the
// netlink implementation of a network address.
type netlinkAddr struct {
	addr *netlink.Addr
}

// IP (ConfigSourceAddr) is a simple property accessor.
func (a *netlinkAddr) IP() net.IP {
	return a.addr.IP
}

// IPNet (ConfigSourceAddr) is a simple property accessor.
func (a *netlinkAddr) IPNet() *net.IPNet {
	return a.addr.IPNet
}

// IsSecondary (ConfigSourceAddr) uses the IFA_F_SECONDARY flag to return
// whether this address is not the primary one for the NIC.
func (a *netlinkAddr) IsSecondary() bool {
	return a.addr.Flags&unix.IFA_F_SECONDARY > 0
}

// String (ConfigSourceAddr) is a simple property accessor.
func (a *netlinkAddr) String() string {
	return a.addr.String()
}

// netlinkNIC implements ConfigSourceNIC by wrapping a netlink Link.
type netlinkNIC struct {
	nic      netlink.Link
	getAddrs func(netlink.Link) ([]netlink.Addr, error)
}

// Name returns the name of the device.
func (n netlinkNIC) Name() string {
	return n.nic.Attrs().Name
}

// Type returns the interface type of the device.
func (n netlinkNIC) Type() LinkLayerDeviceType {
	switch n.nic.Type() {
	case "bridge":
		return BridgeDevice
	case "vlan":
		return VLAN8021QDevice
	case "bond":
		return BondDevice
	case "vxlan":
		return VXLANDevice
	}

	if n.nic.Attrs().Flags&net.FlagLoopback > 0 {
		return LoopbackDevice
	}

	// See comment on super-method.
	// This is incorrect for veth, tuntap, macvtap et al.
	return EthernetDevice
}

// Index returns the index of the device.
func (n netlinkNIC) Index() int {
	return n.nic.Attrs().Index
}

// HardwareAddr returns the hardware address of the device.
func (n netlinkNIC) HardwareAddr() net.HardwareAddr {
	return n.nic.Attrs().HardwareAddr
}

// Addresses returns all IP addresses associated with the device.
func (n netlinkNIC) Addresses() ([]ConfigSourceAddr, error) {
	rawAddrs, err := n.getAddrs(n.nic)
	if err != nil {
		return nil, errors.Capture(err)
	}

	addrs := make([]ConfigSourceAddr, len(rawAddrs))
	for i := range rawAddrs {
		addrs[i] = &netlinkAddr{addr: &rawAddrs[i]}
	}
	return addrs, nil
}

// MTU returns the maximum transmission unit for the device.
func (n netlinkNIC) MTU() int {
	return n.nic.Attrs().MTU
}

// IsUp returns true if the interface is in the "up" state.
func (n netlinkNIC) IsUp() bool {
	return n.nic.Attrs().Flags&net.FlagUp > 0
}

type netlinkConfigSource struct {
	sysClassNetPath string
	linkList        func() ([]netlink.Link, error)
}

// Interfaces returns the network interfaces on the machine.
func (s *netlinkConfigSource) Interfaces() ([]ConfigSourceNIC, error) {
	links, err := s.linkList()
	if err != nil {
		return nil, errors.Capture(err)
	}

	getAddrs := func(l netlink.Link) ([]netlink.Addr, error) {
		return netlink.AddrList(l, netlink.FAMILY_ALL)
	}

	nics := make([]ConfigSourceNIC, len(links))
	for i := range links {
		nics[i] = &netlinkNIC{
			nic:      links[i],
			getAddrs: getAddrs,
		}
	}
	return nics, nil
}

// OvsManagedBridges implements NetworkConfigSource.
func (*netlinkConfigSource) OvsManagedBridges() (set.Strings, error) {
	return OvsManagedBridges()
}

// DefaultRoute implements NetworkConfigSource.
func (*netlinkConfigSource) DefaultRoute() (net.IP, string, error) {
	return GetDefaultRoute()
}

// GetBridgePorts implements NetworkConfigSource.
func (s *netlinkConfigSource) GetBridgePorts(bridgeName string) []string {
	return GetBridgePorts(s.sysClassNetPath, bridgeName)
}

// DefaultConfigSource returns a NetworkConfigSource backed by the
// netlink library, to be used with GetObservedNetworkConfig().
func DefaultConfigSource() ConfigSource {
	return &netlinkConfigSource{
		sysClassNetPath: SysClassNetPath,
		linkList:        netlink.LinkList,
	}
}
