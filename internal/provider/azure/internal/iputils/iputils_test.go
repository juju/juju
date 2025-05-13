// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iputils_test

import (
	"fmt"
	"net"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/azure/internal/iputils"
	"github.com/juju/juju/internal/testing"
)

type iputilsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&iputilsSuite{})

func (*iputilsSuite) TestNextSubnetIP(c *tc.C) {
	assertNextSubnetIP(c, "10.0.0.0/8", nil, "10.0.0.4")
	assertNextSubnetIP(c, "10.0.0.0/8", []string{"10.0.0.1"}, "10.0.0.4")
	assertNextSubnetIP(c, "10.0.0.0/8", []string{"10.0.0.1", "10.0.0.4"}, "10.0.0.5")
}

func (*iputilsSuite) TestNextSubnetIPErrors(c *tc.C) {
	// The subnet is too small to have any non-reserved addresses.
	assertNextSubnetIPError(
		c,
		"10.1.2.0/30",
		nil,
		"no addresses available in 10.1.2.0/30",
	)

	// All addresses in use.
	var addresses []string
	for i := 1; i < 255; i++ {
		addr := fmt.Sprintf("10.0.0.%d", i)
		addresses = append(addresses, addr)
	}
	assertNextSubnetIPError(
		c, "10.0.0.0/24", addresses,
		"no addresses available in 10.0.0.0/24",
	)
}

func (*iputilsSuite) TestNthSubnetIP(c *tc.C) {
	assertNthSubnetIP(c, "10.0.0.0/8", 0, "10.0.0.4")
	assertNthSubnetIP(c, "10.0.0.0/8", 1, "10.0.0.5")
	assertNthSubnetIP(c, "10.0.0.0/29", 0, "10.0.0.4")
	assertNthSubnetIP(c, "10.0.0.0/29", 1, "10.0.0.5")
	assertNthSubnetIP(c, "10.0.0.0/29", 2, "10.0.0.6")
	assertNthSubnetIP(c, "10.0.0.0/29", 3, "") // all bits set, broadcast
	assertNthSubnetIP(c, "10.1.2.0/30", 0, "")
}

func assertNextSubnetIP(c *tc.C, ipnetString string, inuseStrings []string, expectedString string) {
	ipnet := parseIPNet(c, ipnetString)
	inuse := parseIPs(c, inuseStrings...)
	next, err := iputils.NextSubnetIP(ipnet, inuse)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next.String(), tc.Equals, expectedString)
}

func assertNextSubnetIPError(c *tc.C, ipnetString string, inuseStrings []string, expect string) {
	ipnet := parseIPNet(c, ipnetString)
	inuse := parseIPs(c, inuseStrings...)
	_, err := iputils.NextSubnetIP(ipnet, inuse)
	c.Assert(err, tc.ErrorMatches, expect)
}

func assertNthSubnetIP(c *tc.C, ipnetString string, n int, expectedString string) {
	ipnet := parseIPNet(c, ipnetString)
	ip := iputils.NthSubnetIP(ipnet, n)
	if expectedString == "" {
		c.Assert(ip, tc.IsNil)
	} else {
		c.Assert(ip, tc.NotNil)
		c.Assert(ip.String(), tc.Equals, expectedString)
	}
}

func parseIPs(c *tc.C, ipStrings ...string) []net.IP {
	ips := make([]net.IP, len(ipStrings))
	for i, ipString := range ipStrings {
		ip := net.ParseIP(ipString)
		c.Assert(ip, tc.NotNil, tc.Commentf("failed to parse IP %q", ipString))
		ips[i] = ip
	}
	return ips
}

func parseIPNet(c *tc.C, cidr string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(cidr)
	c.Assert(err, tc.ErrorIsNil)
	return ipnet
}
