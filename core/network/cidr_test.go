// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
)

type CIDRSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&CIDRSuite{})

func (s *CIDRSuite) TestCIDRAddressType(c *gc.C) {
	tests := []struct {
		descr  string
		CIDR   string
		exp    network.AddressType
		expErr string
	}{
		{
			descr: "IPV4 CIDR",
			CIDR:  "10.0.0.0/24",
			exp:   network.IPv4Address,
		},
		{
			descr: "IPV6 CIDR",
			CIDR:  "2001:0DB8::/32",
			exp:   network.IPv6Address,
		},
		{
			descr: "IPV6 with 4in6 prefix",
			CIDR:  "0:0:0:0:0:ffff:c0a8:2a00/120",
			exp:   network.IPv6Address,
		},
		{
			descr:  "bogus CIDR",
			CIDR:   "catastrophe",
			expErr: regexp.QuoteMeta(`parsing CIDR "catastrophe"`) + ".*",
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.descr)
		got, err := network.CIDRAddressType(t.CIDR)
		if t.expErr != "" {
			c.Check(got, gc.Equals, network.AddressType(""))
			c.Check(err, gc.ErrorMatches, t.expErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(got, gc.Equals, t.exp)
		}
	}
}

func (s *CIDRSuite) TestNetworkCIDRFromIPAndMask(c *gc.C) {
	specs := []struct {
		descr   string
		ip      net.IP
		mask    net.IPMask
		expCIDR string
	}{
		{
			descr:   "nil ip",
			mask:    net.IPv4Mask(0, 0, 0, 255),
			expCIDR: "",
		},
		{
			descr:   "nil mask",
			ip:      net.ParseIP("10.1.2.42"),
			expCIDR: "",
		},
		{
			descr:   "network IP",
			ip:      net.ParseIP("10.1.0.0"),
			mask:    net.IPv4Mask(255, 255, 0, 0),
			expCIDR: "10.1.0.0/16",
		},
		{
			descr:   "host IP",
			ip:      net.ParseIP("10.1.2.42"),
			mask:    net.IPv4Mask(255, 255, 255, 0),
			expCIDR: "10.1.2.0/24",
		},
	}

	for i, spec := range specs {
		c.Logf("%d: %s", i, spec.descr)
		gotCIDR := network.NetworkCIDRFromIPAndMask(spec.ip, spec.mask)
		c.Assert(gotCIDR, gc.Equals, spec.expCIDR)
	}
}

func (s *CIDRSuite) TestParseCIDRError(c *gc.C) {
	_, err := network.ParseCIDR("192.168.0.0/12")
	c.Assert(err, gc.ErrorMatches, `CIDR "192.168.0.0/12" is not valid`)
}

func (s *CIDRSuite) TestParseCIDR(c *gc.C) {
	for _, cidrStr := range []string{
		"10.1.2.0/24",
		"2001:db8::/32",
	} {
		got, err := network.ParseCIDR(cidrStr)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got.String(), gc.Equals, cidrStr)
	}
}

func (*CIDRSuite) TestAddressRangeForCIDR(c *gc.C) {
	specs := []struct {
		cidr     string
		expFirst string
		expLast  string
	}{
		{
			cidr:     "10.20.30.0/24",
			expFirst: "10.20.30.0",
			expLast:  "10.20.30.255",
		},
		{
			cidr:     "10.20.28.0/22",
			expFirst: "10.20.28.0",
			expLast:  "10.20.31.255",
		},
		{
			cidr:     "192.168.0.0/13",
			expFirst: "192.168.0.0",
			expLast:  "192.175.255.255",
		},
		{
			cidr:     "10.1.2.42/32",
			expFirst: "10.1.2.42",
			expLast:  "10.1.2.42",
		},
		{
			cidr:     "2001:db8::/32",
			expFirst: "2001:db8::",
			expLast:  "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		},
		{
			cidr:     "2001:db8:85a3::8a2e:370:7334/128",
			expFirst: "2001:db8:85a3::8a2e:370:7334",
			expLast:  "2001:db8:85a3::8a2e:370:7334",
		},
	}

	for i, spec := range specs {
		c.Logf("%d. check that range for %q is [%s, %s]", i, spec.cidr, spec.expFirst, spec.expLast)
		cidr, err := network.ParseCIDR(spec.cidr)
		c.Assert(err, jc.ErrorIsNil)
		gotFirst, gotLast := cidr.AddressRange()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotFirst.String(), gc.Equals, spec.expFirst)
		c.Assert(gotLast.String(), gc.Equals, spec.expLast)
	}
}
