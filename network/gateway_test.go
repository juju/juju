// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"net"
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type GatewaySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&GatewaySuite{})

func (s *GatewaySuite) TestDefaultRouteOnMachine(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("skipping default route on-machine test on non-linux")
	}
	s.PatchValue(network.LaunchIpRouteShow, network.LaunchIpRouteShowReal)
	_, _, err := network.GetDefaultRoute()
	c.Check(err, gc.IsNil)
}

func (s *GatewaySuite) TestDefaultRouteLinuxSimple(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, gc.IsNil)
	c.Check(ip, gc.DeepEquals, net.ParseIP("10.0.0.1"))
	c.Check(dev, gc.Equals, "wlp2s0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxTwoRoutes(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 dev lxdbr0 metric 700\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, gc.IsNil)
	c.Check(ip, gc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, gc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoMetric(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, gc.IsNil)
	c.Check(ip, gc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, gc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoGW(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, gc.IsNil)
	c.Check(ip, gc.IsNil)
	c.Check(dev, gc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoDev(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 metric 500\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, gc.IsNil)
	c.Check(ip, gc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, gc.Equals, "")
}

func (s *GatewaySuite) TestDefaultRouteLinuxError(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "", fmt.Errorf("no can do")
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Check(ip, gc.IsNil)
	c.Check(dev, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "no can do")
}

func (s *GatewaySuite) TestDefaultRouteLinuxWrongOutput(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "linux")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric chewbacca\n" +
			"default dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Check(ip, gc.IsNil)
	c.Check(dev, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, ".*chewbacca.*")
}

// We don't return 'not supported' error, we simply give no output
func (s *GatewaySuite) TestDefaultRouteWindowsEmpty(c *gc.C) {
	s.PatchValue(network.SimulatedOS, "windows")
	s.PatchValue(network.LaunchIpRouteShow, func() (string, error) {
		return "", fmt.Errorf("why someone calls me?")
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Check(ip, gc.IsNil)
	c.Check(dev, gc.Equals, "")
	c.Check(err, gc.IsNil)
}
