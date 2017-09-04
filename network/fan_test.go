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
	input := "172.31.0.0/16:253.0.0.0/8"
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
	input := "172.31.0.0/16:253.0.0.0/8;172.32.0.0/16:254.0.0.0/8"
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
	config, err := network.ParseFanConfig("172.31.0.0/16:253.0.0.0/8;foobar")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid FAN config entry:.*")

	// Invalid IP address.
	config, err = network.ParseFanConfig("172.31.0.0/16:253.0.0.0/8;333.122.142.17/18:1.2.3.0/24")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid address in FAN config:.*")
	config, err = network.ParseFanConfig("172.31.0.0/16:253.0.0.0/8;1.2.3.0/24:333.122.142.17/18")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid address in FAN config:.*")

	// Underlay mask smaller than overlay.
	config, err = network.ParseFanConfig("1.0.0.0/8:2.0.0.0/16")
	c.Check(config, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid FAN config, underlay mask must be larger than overlay:.*")
}
