package network

import (
	"net"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vishvananda/netlink"
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

	// If we have get value, return it.
	link.linkType = "bond"
	c.Check(nic.Type(), gc.Equals, BondInterface)

	// Infer loopback from flags.
	link.linkType = ""
	link.flags = net.FlagUp | net.FlagLoopback
	c.Check(nic.Type(), gc.Equals, LoopbackInterface)

	// Default to ethernet otherwise.
	link.flags = net.FlagUp | net.FlagBroadcast | net.FlagMulticast
	c.Check(nic.Type(), gc.Equals, EthernetInterface)
}

func (s *sourceNetlinkSuite) TestNetlinkNICAddrs(c *gc.C) {
	raw, err := netlink.ParseAddr("192.168.20.1/24")
	c.Assert(err, jc.ErrorIsNil)

	getAddrs := func(link netlink.Link) ([]netlink.Addr, error) {
		// Check that we called correctly passing the inner nic.
		c.Assert(link.Attrs().Name, gc.Equals, "eno3")
		return []netlink.Addr{*raw}, nil
	}

	nic := netlinkNIC{
		nic:      &stubLink{},
		getAddrs: getAddrs,
	}

	addrs, err := nic.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 1)
	c.Assert(addrs[0].String(), gc.Equals, "192.168.20.1/24")
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
