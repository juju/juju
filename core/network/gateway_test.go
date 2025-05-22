// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net"
	"os/exec"
	"runtime"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

type GatewaySuite struct {
	network.BaseSuite
}

func TestGatewaySuite(t *testing.T) {
	tc.Run(t, &GatewaySuite{})
}

func (s *GatewaySuite) TestDefaultRouteOnMachine(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("skipping default route on-machine test on non-linux")
	}

	// This just runs "ip" from /sbin directly, as IsolationSuite
	// causes it not to be found in PATH.
	s.PatchRunIPRouteShow(func() (string, error) {
		output, err := exec.Command("/sbin/ip", "route", "show").CombinedOutput()
		if err != nil {
			return "", err
		}
		return string(output), nil
	})

	_, _, err := network.GetDefaultRoute()
	c.Check(err, tc.IsNil)
}

func (s *GatewaySuite) TestDefaultRouteLinuxSimple(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, tc.IsNil)
	c.Check(ip, tc.DeepEquals, net.ParseIP("10.0.0.1"))
	c.Check(dev, tc.Equals, "wlp2s0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxTwoRoutes(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 dev lxdbr0 metric 700\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, tc.IsNil)
	c.Check(ip, tc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, tc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoMetric(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, tc.IsNil)
	c.Check(ip, tc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, tc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoGW(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, tc.IsNil)
	c.Check(ip, tc.IsNil)
	c.Check(dev, tc.Equals, "lxdbr0")
}

func (s *GatewaySuite) TestDefaultRouteLinuxNoDev(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric 800\n" +
			"default via 10.100.1.10 metric 500\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Assert(err, tc.IsNil)
	c.Check(ip, tc.DeepEquals, net.ParseIP("10.100.1.10"))
	c.Check(dev, tc.Equals, "")
}

func (s *GatewaySuite) TestDefaultRouteLinuxError(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "", errors.Errorf("no can do")
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Check(ip, tc.IsNil)
	c.Check(dev, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "no can do")
}

func (s *GatewaySuite) TestDefaultRouteLinuxWrongOutput(c *tc.C) {
	s.PatchGOOS("linux")
	s.PatchRunIPRouteShow(func() (string, error) {
		return "default via 10.0.0.1 dev wlp2s0 proto static metric chewbacca\n" +
			"default dev lxdbr0\n" +
			"10.0.0.0/24 dev wlp2s0 proto kernel scope link src 10.0.0.66 metric 600\n" +
			"10.100.1.0/24 dev lxdbr0 proto kernel scope link src 10.100.1.1\n", nil
	})
	ip, dev, err := network.GetDefaultRoute()
	c.Check(ip, tc.IsNil)
	c.Check(dev, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, ".*chewbacca.*")
}
