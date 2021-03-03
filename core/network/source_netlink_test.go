package network

import (
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
