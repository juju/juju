// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

	"github.com/juju/errors"
	"github.com/vishvananda/netlink"
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
func (n netlinkNIC) Type() InterfaceType {
	switch n.nic.Type() {
	case "bridge":
		return BridgeInterface
	case "vlan":
		return VLAN_8021QInterface
	case "bond":
		return BondInterface
	}

	if n.nic.Attrs().Flags&net.FlagLoopback > 0 {
		return LoopbackInterface
	}

	return EthernetInterface
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
		return nil, errors.Trace(err)
	}

	addrs := make([]ConfigSourceAddr, len(rawAddrs))
	for i := range rawAddrs {
		addrs[i] = &netlinkAddr{&rawAddrs[i]}
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
