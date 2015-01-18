// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type InterfaceInfoSuite struct {
	info []network.InterfaceInfo
}

var _ = gc.Suite(&InterfaceInfoSuite{})

func (s *InterfaceInfoSuite) SetUpTest(c *gc.C) {
	s.info = []network.InterfaceInfo{
		{VLANTag: 1, DeviceIndex: 0, InterfaceName: "eth0"},
		{VLANTag: 0, DeviceIndex: 1, InterfaceName: "eth1"},
		{VLANTag: 42, DeviceIndex: 2, InterfaceName: "br2"},
		{ConfigType: network.ConfigDHCP, NoAutoStart: true},
		{Address: network.NewAddress("0.1.2.3", network.ScopeUnknown)},
		{DNSServers: network.NewAddresses("1.1.1.1", "2.2.2.2")},
		{GatewayAddress: network.NewAddress("4.3.2.1", network.ScopeUnknown)},
		{ExtraConfig: map[string]string{
			"foo": "bar",
			"baz": "nonsense",
		}},
	}
}

func (s *InterfaceInfoSuite) TestActualInterfaceName(c *gc.C) {
	c.Check(s.info[0].ActualInterfaceName(), gc.Equals, "eth0.1")
	c.Check(s.info[1].ActualInterfaceName(), gc.Equals, "eth1")
	c.Check(s.info[2].ActualInterfaceName(), gc.Equals, "br2.42")
}

func (s *InterfaceInfoSuite) TestIsVirtual(c *gc.C) {
	c.Check(s.info[0].IsVirtual(), jc.IsTrue)
	c.Check(s.info[1].IsVirtual(), jc.IsFalse)
	c.Check(s.info[2].IsVirtual(), jc.IsTrue)
}

func (s *InterfaceInfoSuite) TestIsVLAN(c *gc.C) {
	c.Check(s.info[0].IsVLAN(), jc.IsTrue)
	c.Check(s.info[1].IsVLAN(), jc.IsFalse)
	c.Check(s.info[2].IsVLAN(), jc.IsTrue)
}

func (s *InterfaceInfoSuite) TestAdditionalFields(c *gc.C) {
	c.Check(s.info[3].ConfigType, gc.Equals, network.ConfigDHCP)
	c.Check(s.info[3].NoAutoStart, jc.IsTrue)
	c.Check(s.info[4].Address, jc.DeepEquals, network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Check(s.info[5].DNSServers, jc.DeepEquals, network.NewAddresses("1.1.1.1", "2.2.2.2"))
	c.Check(s.info[6].GatewayAddress, jc.DeepEquals, network.NewAddress("4.3.2.1", network.ScopeUnknown))
	c.Check(s.info[7].ExtraConfig, jc.DeepEquals, map[string]string{
		"foo": "bar",
		"baz": "nonsense",
	})
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
