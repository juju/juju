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
	addr := network.IPAddress{netip.MustParseAddr("192.168.1.1")}
	msb, lsb := addr.AsInts()
	c.Assert(msb, gc.Equals, uint64(0))
	c.Assert(lsb, gc.Equals, uint64(0xc0a80101))
}

func (s *IPAddressSuite) TestIPv6AddressSplit(c *gc.C) {
	addr := network.IPAddress{netip.MustParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7666")}
	msb, lsb := addr.AsInts()
	c.Assert(msb, gc.Equals, uint64(0x20010db885a30000))
	c.Assert(lsb, gc.Equals, uint64(0x8a2e03707666))
}
