// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net/netip"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
)

type IPAddressSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&IPAddressSuite{})

func (s *IPAddressSuite) TestIPv4AddressSplit(c *gc.C) {
	addr := network.IPAddress{netip.MustParseAddr("192.168.0.0")}
	msb, lsb := addr.AsInts()
	c.Assert(msb, gc.Equals, uint64(0))
	c.Assert(lsb, gc.Equals, uint64(0xc0a80000))
}

func (s *IPAddressSuite) TestIPv6AddressSplit(c *gc.C) {
	addr := network.IPAddress{netip.MustParseAddr("fd7a:115c:a1e0:ab12:4843:cd96:626b:430b")}
	msb, lsb := addr.AsInts()
	c.Assert(msb, gc.Equals, uint64(0xfd7a115ca1e0ab12))
	c.Assert(lsb, gc.Equals, uint64(0x4843cd96626b430b))
}
