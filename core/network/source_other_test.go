// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type sourceOtherSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sourceOtherSuite{})

func (s *sourceOtherSuite) TestNewNetAddr(c *gc.C) {
	nic, err := NewNetAddr("192.168.20.1/24")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nic.String(), gc.Equals, "192.168.20.1/24")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IPNet(), gc.NotNil)
	c.Check(nic.IPNet().String(), gc.Equals, "192.168.20.0/24")

	nic, err = NewNetAddr("192.168.20.1")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nic.String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "192.168.20.1")
	c.Assert(nic.IPNet(), gc.IsNil)

	nic, err = NewNetAddr("fe80::5054:ff:fedd:eef0/64")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nic.String(), gc.Equals, "fe80::5054:ff:fedd:eef0/64")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IPNet(), gc.NotNil)
	c.Check(nic.IPNet().String(), gc.Equals, "fe80::/64")

	nic, err = NewNetAddr("fe80::5054:ff:fedd:eef0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nic.String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IP(), gc.NotNil)
	c.Check(nic.IP().String(), gc.Equals, "fe80::5054:ff:fedd:eef0")
	c.Assert(nic.IPNet(), gc.IsNil)

	nic, err = NewNetAddr("y u no parse")
	c.Assert(err, gc.ErrorMatches, `unable to parse IP address "y u no parse"`)
}
