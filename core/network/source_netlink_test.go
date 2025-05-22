// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package network

import (
	"net"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/juju/juju/internal/testhelpers"
)

type sourceNetlinkSuite struct {
	testhelpers.IsolationSuite
}

func TestSourceNetlinkSuite(t *stdtesting.T) {
	tc.Run(t, &sourceNetlinkSuite{})
}

func (s *sourceNetlinkSuite) TestNetlinkAddr(c *tc.C) {
	raw, err := netlink.ParseAddr("192.168.20.1/24")
	c.Assert(err, tc.ErrorIsNil)
	addr := &netlinkAddr{raw}

	c.Check(addr.String(), tc.Equals, "192.168.20.1/24")
	c.Assert(addr.IP(), tc.NotNil)
	c.Check(addr.IP().String(), tc.Equals, "192.168.20.1")
	c.Assert(addr.IPNet(), tc.NotNil)
	c.Check(addr.IPNet().String(), tc.Equals, "192.168.20.1/24")

	raw, err = netlink.ParseAddr("fe80::5054:ff:fedd:eef0/64")
	c.Assert(err, tc.ErrorIsNil)
	addr = &netlinkAddr{raw}

	c.Check(addr.String(), tc.Equals, "fe80::5054:ff:fedd:eef0/64")
	c.Assert(addr.IP(), tc.NotNil)
	c.Check(addr.IP().String(), tc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(addr.IPNet(), tc.NotNil)
	c.Check(addr.IPNet().String(), tc.Equals, "fe80::5054:ff:fedd:eef0/64")

	c.Check(addr.IsSecondary(), tc.IsFalse)

	addr.addr.Flags = addr.addr.Flags | unix.IFA_F_SECONDARY
	c.Check(addr.IsSecondary(), tc.IsTrue)
}

func (s *sourceNetlinkSuite) TestNetlinkAttrs(c *tc.C) {
	link := &stubLink{flags: net.FlagUp}
	nic := &netlinkNIC{nic: link}

	c.Check(nic.MTU(), tc.Equals, 1500)
	c.Check(nic.Name(), tc.Equals, "eno3")
	c.Check(nic.IsUp(), tc.IsTrue)
	c.Check(nic.Index(), tc.Equals, 3)
	c.Check(nic.HardwareAddr(), tc.DeepEquals, net.HardwareAddr{})
}

func (s *sourceNetlinkSuite) TestNetlinkNICType(c *tc.C) {
	link := &stubLink{}
	nic := &netlinkNIC{nic: link}

	// Known types.
	link.linkType = "bridge"
	c.Check(nic.Type(), tc.Equals, BridgeDevice)

	link.linkType = "bond"
	c.Check(nic.Type(), tc.Equals, BondDevice)

	link.linkType = "vxlan"
	c.Check(nic.Type(), tc.Equals, VXLANDevice)

	// Infer loopback from flags.
	link.linkType = ""
	link.flags = net.FlagUp | net.FlagLoopback
	c.Check(nic.Type(), tc.Equals, LoopbackDevice)

	// Default to ethernet otherwise.
	link.flags = net.FlagUp | net.FlagBroadcast | net.FlagMulticast
	c.Check(nic.Type(), tc.Equals, EthernetDevice)
}

func (s *sourceNetlinkSuite) TestNetlinkNICAddrs(c *tc.C) {
	raw1, err := netlink.ParseAddr("192.168.20.1/24")
	c.Assert(err, tc.ErrorIsNil)

	raw2, err := netlink.ParseAddr("fe80::5054:ff:fedd:eef0/64")
	c.Assert(err, tc.ErrorIsNil)

	getAddrs := func(link netlink.Link) ([]netlink.Addr, error) {
		// Check that we called correctly passing the inner nic.
		c.Assert(link.Attrs().Name, tc.Equals, "eno3")
		return []netlink.Addr{*raw1, *raw2}, nil
	}

	nic := netlinkNIC{
		nic:      &stubLink{},
		getAddrs: getAddrs,
	}

	addrs, err := nic.Addresses()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(addrs, tc.HasLen, 2)
	c.Assert(addrs[0].String(), tc.Equals, "192.168.20.1/24")
	c.Assert(addrs[1].String(), tc.Equals, "fe80::5054:ff:fedd:eef0/64")
}

func (s *sourceNetlinkSuite) TestNetlinkSourceInterfaces(c *tc.C) {
	link1 := &stubLink{linkType: "bridge"}
	link2 := &stubLink{linkType: "bond"}

	source := &netlinkConfigSource{
		linkList: func() ([]netlink.Link, error) { return []netlink.Link{link1, link2}, nil },
	}

	nics, err := source.Interfaces()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 2)
	c.Check(nics[0].Type(), tc.Equals, BridgeDevice)
	c.Check(nics[1].Type(), tc.Equals, BondDevice)
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
