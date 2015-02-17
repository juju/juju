// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type InfoSuite struct {
	info []network.Info
}

var _ = gc.Suite(&InfoSuite{})

func (n *InfoSuite) SetUpTest(c *gc.C) {
	n.info = []network.Info{
		{VLANTag: 1, DeviceIndex: 0, InterfaceName: "eth0"},
		{VLANTag: 0, DeviceIndex: 1, InterfaceName: "eth1"},
		{VLANTag: 42, DeviceIndex: 2, InterfaceName: "br2"},
	}
}

func (n *InfoSuite) TestActualInterfaceName(c *gc.C) {
	c.Check(n.info[0].ActualInterfaceName(), gc.Equals, "eth0.1")
	c.Check(n.info[1].ActualInterfaceName(), gc.Equals, "eth1")
	c.Check(n.info[2].ActualInterfaceName(), gc.Equals, "br2.42")
}

func (n *InfoSuite) TestIsVirtual(c *gc.C) {
	c.Check(n.info[0].IsVirtual(), jc.IsTrue)
	c.Check(n.info[1].IsVirtual(), jc.IsFalse)
	c.Check(n.info[2].IsVirtual(), jc.IsTrue)
}

func (n *InfoSuite) TestIsVLAN(c *gc.C) {
	c.Check(n.info[0].IsVLAN(), jc.IsTrue)
	c.Check(n.info[1].IsVLAN(), jc.IsFalse)
	c.Check(n.info[2].IsVLAN(), jc.IsTrue)
}

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (*NetworkSuite) TestInitializeFromConfig(c *gc.C) {
	c.Check(network.GetPreferIPv6(), jc.IsFalse)

	envConfig := testing.CustomEnvironConfig(c, testing.Attrs{
		"prefer-ipv6": true,
	})
	network.InitializeFromConfig(envConfig)
	c.Check(network.GetPreferIPv6(), jc.IsTrue)

	envConfig = testing.CustomEnvironConfig(c, testing.Attrs{
		"prefer-ipv6": false,
	})
	network.InitializeFromConfig(envConfig)
	c.Check(network.GetPreferIPv6(), jc.IsFalse)
}

func (s *NetworkSuite) TestFilterLXCAddresses(c *gc.C) {
	lxcFakeNetConfig := filepath.Join(c.MkDir(), "lxc-net")
	netConf := []byte(`
  # comments ignored
LXC_BR= ignored
LXC_ADDR = "fooo"
 LXC_BRIDGE = " foobar " # detected, spaces stripped
anything else ignored
LXC_BRIDGE="ignored"`[1:])
	err := ioutil.WriteFile(lxcFakeNetConfig, netConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		c.Assert(name, gc.Equals, "foobar")
		return []net.Addr{
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
			// Try a CIDR 10.0.3.5/24 as well.
			&net.IPNet{IP: net.IPv4(10, 0, 3, 5), Mask: net.IPv4Mask(255, 255, 255, 0)},
		}, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	inputAddresses := network.NewAddresses(
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"10.0.3.1", // filtered (directly as IP)
		"10.0.3.3", // filtered (by the 10.0.3.5/24 CIDR)
		"10.0.3.5", // filtered (directly)
		"10.0.3.4", // filtered (directly)
		"192.168.123.42",
	)
	filteredAddresses := network.NewAddresses(
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"192.168.123.42",
	)
	c.Assert(network.FilterLXCAddresses(inputAddresses), jc.DeepEquals, filteredAddresses)
}
