// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type FanConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&FanConfigSuite{})

func (*FanConfigSuite) TestFanConfigParseEmpty(c *gc.C) {
	config, err := network.ParseFanConfig("")
	c.Check(config, gc.IsNil)
	c.Check(err, jc.ErrorIsNil)
}

func (*FanConfigSuite) TestFanConfigParseSingle(c *gc.C) {
	input := "172.31.0.0/16=253.0.0.0/8"
	config, err := network.ParseFanConfig(input)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config, gc.HasLen, 1)
	_, underlay, _ := net.ParseCIDR("172.31.0.0/16")
	_, overlay, _ := net.ParseCIDR("253.0.0.0/8")
	c.Check(config[0].Underlay, gc.DeepEquals, underlay)
	c.Check(config[0].Overlay, gc.DeepEquals, overlay)
	c.Check(config.String(), gc.Equals, input)
}

func (*FanConfigSuite) TestFanConfigParseMultiple(c *gc.C) {
	input := "172.31.0.0/16=253.0.0.0/8 172.32.0.0/16=254.0.0.0/8"
	config, err := network.ParseFanConfig(input)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config, gc.HasLen, 2)
	_, underlay0, _ := net.ParseCIDR("172.31.0.0/16")
	_, overlay0, _ := net.ParseCIDR("253.0.0.0/8")
	c.Check(config[0].Underlay, gc.DeepEquals, underlay0)
	c.Check(config[0].Overlay, gc.DeepEquals, overlay0)
	_, underlay1, _ := net.ParseCIDR("172.32.0.0/16")
	_, overlay1, _ := net.ParseCIDR("254.0.0.0/8")
	c.Check(config[1].Underlay, gc.DeepEquals, underlay1)
	c.Check(config[1].Overlay, gc.DeepEquals, overlay1)
	c.Check(config.String(), gc.Equals, input)
}

func (*FanConfigSuite) TestFanConfigErrors(c *gc.C) {
	// Colonless garbage.
	config, err := network.ParseFanConfig("172.31.0.0/16=253.0.0.0/8 foobar")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid FAN config entry:.*")

	// Invalid IP address.
	config, err = network.ParseFanConfig("172.31.0.0/16=253.0.0.0/8 333.122.142.17/18=1.2.3.0/24")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid address in FAN config:.*")
	config, err = network.ParseFanConfig("172.31.0.0/16=253.0.0.0/8 1.2.3.0/24=333.122.142.17/18")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid address in FAN config:.*")

	// Underlay mask smaller than overlay.
	config, err = network.ParseFanConfig("1.0.0.0/8=2.0.0.0/16")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid FAN config, underlay mask must be larger than overlay:.*")
}

func (*FanConfigSuite) TestCalculateOverlaySegment(c *gc.C) {
	config, err := network.ParseFanConfig("172.31.0.0/16=253.0.0.0/8 10.0.0.0/12=252.0.0.0/7")
	c.Assert(err, jc.ErrorIsNil)

	// Regular case
	net, err := network.CalculateOverlaySegment("172.31.16.0/20", config[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(net, gc.NotNil)
	c.Check(net.String(), gc.Equals, "253.16.0.0/12")

	// Underlay outside of FAN scope
	net, err = network.CalculateOverlaySegment("172.99.16.0/20", config[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(net, gc.IsNil)

	// Garbage in underlay
	net, err = network.CalculateOverlaySegment("something", config[0])
	c.Check(net, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid CIDR address: something")

	// Funky case
	// We have overlay 252.0.0.0/7 underlay 10.0.0.0/12 and local underlay 10.2.224.0/19
	// We need to transplant 19-12=7 bits out of local underlay to overlay
	// 10.2.224.0/19 is 00001010.00000010.11100000.00000000
	//  the bits we want to cut      **** ***
	// 252.0.0.0/7   is 11111100.00000000.00000000.00000000
	// After transplanting those 7 bits we get
	//                  11111100.01011100.00000000.00000000
	//                         * ******
	// which is 252.92.0.0/14
	net, err = network.CalculateOverlaySegment("10.2.224.0/19", config[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(net, gc.NotNil)
	c.Check(net.String(), gc.Equals, "252.92.0.0/14")
}

func (*FanConfigSuite) TestCalculateOverlaySegmentNonIPv4FanAddress(c *gc.C) {
	// Use mapping from smaller IPv6 subnet to larger overlay.
	config, err := network.ParseFanConfig("2001:db8::/16=2001:db7::/8")

	// CalculateOverlaySegment does not support IPv6 addresses.
	_, err = network.CalculateOverlaySegment("2001:db8::/16", config[0])
	c.Assert(err, gc.ErrorMatches, "fan address is not an IPv4 address.")
}
