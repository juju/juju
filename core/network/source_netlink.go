// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"

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
