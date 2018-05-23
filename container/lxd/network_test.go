// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"errors"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	"github.com/juju/juju/network"
)

type networkSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&networkSuite{})

func defaultProfile() *lxdapi.Profile {
	return &lxdapi.Profile{
		Name: "default",
		ProfilePut: lxdapi.ProfilePut{
			Devices: map[string]map[string]string{
				"eth0": {
					"parent":  network.DefaultLXDBridge,
					"type":    "nic",
					"nictype": "bridged",
				},
			},
		},
	}
}

func (s *networkSuite) TestEnsureIPv4NoChange(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	net := &lxdapi.Network{
		NetworkPut: lxdapi.NetworkPut{
			Config: map[string]string{
				"ipv4.address": "10.5.3.1",
			},
		},
	}
	cSvr.EXPECT().GetNetwork("some-net-name").Return(net, lxdtesting.ETag, nil)

	mod, err := lxd.NewServer(cSvr).EnsureIPv4("some-net-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod, jc.IsFalse)
}

func (s *networkSuite) TestEnsureIPv4Modified(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	req := lxdapi.NetworkPut{
		Config: map[string]string{
			"ipv4.address": "auto",
			"ipv4.nat":     "true",
		},
	}
	gomock.InOrder(
		cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(&lxdapi.Network{}, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateNetwork(network.DefaultLXDBridge, req, lxdtesting.ETag).Return(nil),
	)

	mod, err := lxd.NewServer(cSvr).EnsureIPv4(network.DefaultLXDBridge)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod, jc.IsTrue)
}

func (s *networkSuite) TestVerifyDefaultBridgeNetSupportDevicePresent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(&lxdapi.Network{}, "", nil)

	err := lxd.NewServer(cSvr).VerifyDefaultBridge(defaultProfile(), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestVerifyDefaultBridgeNetSupportDeviceNotBridged(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(&lxdapi.Network{}, "", nil)

	profile := defaultProfile()
	profile.Devices["eth0"]["nictype"] = "something else"
	err := lxd.NewServer(cSvr).VerifyDefaultBridge(profile, "")
	c.Assert(err, gc.ErrorMatches, ".*eth0 is not configured as part of a bridge.*")
}

func (s *networkSuite) TestVerifyDefaultBridgeNetSupportIPv6Present(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	net := &lxdapi.Network{
		Name:    network.DefaultLXDBridge,
		Managed: true,
		NetworkPut: lxdapi.NetworkPut{
			Config: map[string]string{
				"ipv6.address": "something-not-nothing",
			},
		},
	}
	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(net, "", nil)

	err := lxd.NewServer(cSvr).VerifyDefaultBridge(defaultProfile(), "")
	c.Assert(err, gc.ErrorMatches, "^juju does not support IPv6((.|\n|\t)*)")
}

func (s *networkSuite) TestVerifyDefaultBridgeNetSupportNoBridge(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl, "network")

	netConf := map[string]string{
		"ipv4.address": "auto",
		"ipv4.nat":     "true",
		"ipv6.address": "none",
		"ipv6.nat":     "false",
	}
	netCreateReq := lxdapi.NetworksPost{
		Name:       network.DefaultLXDBridge,
		Type:       "bridge",
		Managed:    true,
		NetworkPut: lxdapi.NetworkPut{Config: netConf},
	}
	newNet := &lxdapi.Network{
		Name:       network.DefaultLXDBridge,
		Type:       "bridge",
		Managed:    true,
		NetworkPut: lxdapi.NetworkPut{Config: netConf},
	}
	gomock.InOrder(
		cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(nil, "", errors.New("not found")),
		cSvr.EXPECT().CreateNetwork(netCreateReq).Return(nil),
		cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(newNet, "", nil),
		cSvr.EXPECT().UpdateProfile("default", defaultProfile().Writable(), lxdtesting.ETag).Return(nil),
	)

	profile := defaultProfile()
	delete(profile.Devices, "eth0")
	err := lxd.NewServer(cSvr).VerifyDefaultBridge(profile, lxdtesting.ETag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestCheckLXDBridgeConfiguration(c *gc.C) {
	var err error

	valid := func(string) ([]byte, error) {
		return []byte(`
# Whether to setup a new bridge or use an existing one
USE_LXD_BRIDGE="true"

# Bridge name
# This is still used even if USE_LXD_BRIDGE is set to false
# set to an empty value to fully disable
LXD_BRIDGE="lxdbr0"

# Path to an extra dnsmasq configuration file
LXD_CONFILE=""

# DNS domain for the bridge
LXD_DOMAIN="lxd"

# IPv4
## IPv4 address (e.g. 10.0.4.1)
LXD_IPV4_ADDR="10.0.4.1"

## IPv4 netmask (e.g. 255.255.255.0)
LXD_IPV4_NETMASK="255.255.255.0"

## IPv4 network (e.g. 10.0.4.0/24)
LXD_IPV4_NETWORK="10.0.4.1/24"

## IPv4 DHCP range (e.g. 10.0.4.2,10.0.4.254)
LXD_IPV4_DHCP_RANGE="10.0.4.2,10.0.4.254"

## IPv4 DHCP number of hosts (e.g. 250)
LXD_IPV4_DHCP_MAX="253"

## NAT IPv4 traffic
LXD_IPV4_NAT="true"

# IPv6
## IPv6 address (e.g. 2001:470:b368:4242::1)
LXD_IPV6_ADDR=""

## IPv6 CIDR mask (e.g. 64)
LXD_IPV6_MASK=""
LXD_IPV6_NETWORK=""
`), nil
	}

	err = lxd.CheckBridgeConfigFile(valid)
	c.Assert(err, jc.ErrorIsNil)

	noBridge := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="false"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(noBridge)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 not enabled but required
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	badName := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="meshuggahrocks"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(badName)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has a bridge named meshuggahrocks, not lxdbr0
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	noSubnets := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
`), nil
	}
	err = lxd.CheckBridgeConfigFile(noSubnets)
	c.Assert(err.Error(), gc.Equals, `lxdbr0 has no ipv4 or ipv6 subnet enabled
It looks like your lxdbr0 has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	ipv6 := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
LXD_IPV6_ADDR="2001:470:b368:4242::1"
`), nil
	}

	err = lxd.CheckBridgeConfigFile(ipv6)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has IPv6 enabled.
Juju doesn't currently support IPv6.
Disable IPv6 via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)
}
