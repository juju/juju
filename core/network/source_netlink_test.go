// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux
// +build linux

package network

import (
	"net"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	gc "gopkg.in/check.v1"
)

type sourceNetlinkSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sourceNetlinkSuite{})

func (s *sourceNetlinkSuite) TestNetlinkAddr(c *gc.C) {
	raw, err := netlink.ParseAddr("192.168.20.1/24")
	c.Assert(err, jc.ErrorIsNil)
	addr := &netlinkAddr{raw}

	c.Check(addr.String(), gc.Equals, "192.168.20.1/24")
	c.Assert(addr.IP(), gc.NotNil)
	c.Check(addr.IP().String(), gc.Equals, "192.168.20.1")
	c.Assert(addr.IPNet(), gc.NotNil)
	c.Check(addr.IPNet().String(), gc.Equals, "192.168.20.1/24")

	raw, err = netlink.ParseAddr("fe80::5054:ff:fedd:eef0/64")
	c.Assert(err, jc.ErrorIsNil)
	addr = &netlinkAddr{raw}

	c.Check(addr.String(), gc.Equals, "fe80::5054:ff:fedd:eef0/64")
	c.Assert(addr.IP(), gc.NotNil)
	c.Check(addr.IP().String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(addr.IPNet(), gc.NotNil)
	c.Check(addr.IPNet().String(), gc.Equals, "fe80::5054:ff:fedd:eef0/64")

	c.Check(addr.IsSecondary(), jc.IsFalse)

	addr.addr.Flags = addr.addr.Flags | unix.IFA_F_SECONDARY
	c.Check(addr.IsSecondary(), jc.IsTrue)
}

func (s *sourceNetlinkSuite) TestNetlinkAttrs(c *gc.C) {
	link := &stubLink{flags: net.FlagUp}
	nic := &netlinkNIC{nic: link}

	c.Check(nic.MTU(), gc.Equals, 1500)
	c.Check(nic.Name(), gc.Equals, "eno3")
	c.Check(nic.IsUp(), jc.IsTrue)
	c.Check(nic.Index(), gc.Equals, 3)
	c.Check(nic.HardwareAddr(), gc.DeepEquals, net.HardwareAddr{})
}

func (s *sourceNetlinkSuite) TestNetlinkNICType(c *gc.C) {
	link := &stubLink{}
	nic := &netlinkNIC{nic: link}

	// Known types.
	link.linkType = "bridge"
	c.Check(nic.Type(), gc.Equals, BridgeDevice)

	link.linkType = "bond"
	c.Check(nic.Type(), gc.Equals, BondDevice)

	link.linkType = "vxlan"
	c.Check(nic.Type(), gc.Equals, VXLANDevice)

	// Infer loopback from flags.
	link.linkType = ""
	link.flags = net.FlagUp | net.FlagLoopback
	c.Check(nic.Type(), gc.Equals, LoopbackDevice)

	// Default to ethernet otherwise.
	link.flags = net.FlagUp | net.FlagBroadcast | net.FlagMulticast
	c.Check(nic.Type(), gc.Equals, EthernetDevice)
}

func (s *sourceNetlinkSuite) TestNetlinkNICAddrs(c *gc.C) {
	raw1, err := netlink.ParseAddr("192.168.20.1/24")
	c.Assert(err, jc.ErrorIsNil)

	raw2, err := netlink.ParseAddr("fe80::5054:ff:fedd:eef0/64")
	c.Assert(err, jc.ErrorIsNil)

	getAddrs := func(link netlink.Link) ([]netlink.Addr, error) {
		// Check that we called correctly passing the inner nic.
		c.Assert(link.Attrs().Name, gc.Equals, "eno3")
		return []netlink.Addr{*raw1, *raw2}, nil
	}

	nic := netlinkNIC{
		nic:      &stubLink{},
		getAddrs: getAddrs,
	}

	addrs, err := nic.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addrs, gc.HasLen, 2)
	c.Assert(addrs[0].String(), gc.Equals, "192.168.20.1/24")
	c.Assert(addrs[1].String(), gc.Equals, "fe80::5054:ff:fedd:eef0/64")
}

func (s *sourceNetlinkSuite) TestNetlinkSourceInterfaces(c *gc.C) {
	link1 := &stubLink{linkType: "bridge"}
	link2 := &stubLink{linkType: "bond"}

	source := &netlinkConfigSource{
		linkList: func() ([]netlink.Link, error) { return []netlink.Link{link1, link2}, nil },
	}

	nics, err := source.Interfaces()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(nics, gc.HasLen, 2)
	c.Check(nics[0].Type(), gc.Equals, BridgeDevice)
	c.Check(nics[1].Type(), gc.Equals, BondDevice)
}

// stubLink stubs netlink.Link
type stubLink struct {
	linkType string
	flags    net.Flags
}

func (l *stubLink) Attrs() *netlink.LinkAttrs {
	return &netlink.LinkAttrs{
		Index:        3,
		MTU:          1500,
		Name:         "eno3",
		HardwareAddr: net.HardwareAddr{},
		Flags:        l.flags,
	}
}

func (l *stubLink) Type() string {
	return l.linkType
}
