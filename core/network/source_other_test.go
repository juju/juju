// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type sourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sourceSuite{})

func (s *sourceSuite) TestNewNetAddr(c *gc.C) {
	nic := NewNetAddr("192.168.20.1/24")
	c.Check(nic.String(), gc.Equals, "192.168.20.1/24")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IPNet(), gc.NotNil)
	c.Check(nic.IPNet().String(), gc.Equals, "192.168.20.0/24")

	nic = NewNetAddr("192.168.20.1")
	c.Check(nic.String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IPNet(), gc.IsNil)

	nic = NewNetAddr("fe80::5054:ff:fedd:eef0/64")
	c.Check(nic.String(), gc.Equals, "fe80::5054:ff:fedd:eef0/64")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IPNet(), gc.NotNil)
	c.Check(nic.IPNet().String(), gc.Equals, "fe80::/64")

	nic = NewNetAddr("fe80::5054:ff:fedd:eef0")
	c.Check(nic.String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IPNet(), gc.IsNil)
}
