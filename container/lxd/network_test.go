// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

type networkSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&networkSuite{})

func (s *networkSuite) patch() {
	lxd.PatchGenerateVirtualMACAddress(s)
}

func defaultProfileWithNIC() *lxdapi.Profile {
	return &lxdapi.Profile{
		Name: "default",
		ProfilePut: lxdapi.ProfilePut{
			Devices: map[string]map[string]string{
				"eth0": {
					"network": network.DefaultLXDBridge,
					"type":    "nic",
				},
			},
		},
	}
}

func defaultLegacyProfileWithNIC() *lxdapi.Profile {
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
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

	net := &lxdapi.Network{
		NetworkPut: lxdapi.NetworkPut{
			Config: map[string]string{
				"ipv4.address": "10.5.3.1",
			},
		},
	}
	cSvr.EXPECT().GetNetwork("some-net-name").Return(net, lxdtesting.ETag, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	mod, err := jujuSvr.EnsureIPv4("some-net-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod, jc.IsFalse)
}

func (s *networkSuite) TestEnsureIPv4Modified(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

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

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	mod, err := jujuSvr.EnsureIPv4(network.DefaultLXDBridge)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod, jc.IsTrue)
}

func (s *networkSuite) TestGetNICsFromProfile(c *gc.C) {
	lxd.PatchGenerateVirtualMACAddress(s)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cSvr.EXPECT().GetProfile("default").Return(defaultLegacyProfileWithNIC(), lxdtesting.ETag, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	nics, err := jujuSvr.GetNICsFromProfile("default")
	c.Assert(err, jc.ErrorIsNil)

	exp := map[string]map[string]string{
		"eth0": {
			"parent":  network.DefaultLXDBridge,
			"type":    "nic",
			"nictype": "bridged",
			"hwaddr":  "00:16:3e:00:00:00",
		},
	}

	c.Check(nics, gc.DeepEquals, exp)
}

func (s *networkSuite) TestVerifyNetworkDevicePresentValid(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

	net := &lxdapi.Network{
		Name:    network.DefaultLXDBridge,
		Managed: true,
		Type:    "bridge",
	}
	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(net, "", nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(defaultProfileWithNIC(), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestVerifyNetworkDevicePresentValidLegacy(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(&lxdapi.Network{}, "", nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(defaultLegacyProfileWithNIC(), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestVerifyNetworkDeviceMultipleNICsOneValid(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerClustered(ctrl, "cluster-1")

	profile := defaultLegacyProfileWithNIC()
	profile.Devices["eno1"] = profile.Devices["eth0"]
	profile.Devices["eno1"]["parent"] = "valid-net"

	net := &lxdapi.Network{
		Name:    network.DefaultLXDBridge,
		Managed: true,
		NetworkPut: lxdapi.NetworkPut{
			Config: map[string]string{
				"ipv6.address": "something-not-nothing",
			},
		},
	}

	// Random map iteration may or may not cause this call to be made.
	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(net, "", nil).MaxTimes(1)
	cSvr.EXPECT().GetNetwork("valid-net").Return(&lxdapi.Network{}, "", nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(profile, "")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(jujuSvr.LocalBridgeName(), gc.Equals, "valid-net")
}

func (s *networkSuite) TestVerifyNetworkDevicePresentBadNicType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

	profile := defaultLegacyProfileWithNIC()
	profile.Devices["eth0"]["nictype"] = "not-bridge-or-macvlan"

	net := &lxdapi.Network{
		Name:    network.DefaultLXDBridge,
		Managed: true,
		Type:    "not-bridge-or-macvlan",
	}
	cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(net, "", nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(profile, "")
	c.Assert(err, gc.ErrorMatches,
		`profile "default": no network device found with nictype "bridged" or "macvlan"\n`+
			`\tthe following devices were checked: eth0\n`+
			`Note: juju does not support IPv6.\n`+
			`Reconfigure lxd to use a network of type "bridged" or "macvlan", disabling IPv6.`)
}

func (s *networkSuite) TestVerifyNetworkDeviceIPv6Present(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

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

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(defaultLegacyProfileWithNIC(), "")
	c.Assert(err, gc.ErrorMatches,
		`profile "default": juju does not support IPv6. Disable IPv6 in LXD via:\n`+
			`\tlxc network set lxdbr0 ipv6.address none\n`+
			`and run the command again`)
}

func (s *networkSuite) TestVerifyNetworkDeviceNotPresentCreated(c *gc.C) {
	s.patch()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

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
		cSvr.EXPECT().UpdateProfile("default", defaultLegacyProfileWithNIC().Writable(), lxdtesting.ETag).Return(nil),
	)

	profile := defaultLegacyProfileWithNIC()
	delete(profile.Devices, "eth0")

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(profile, lxdtesting.ETag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestVerifyNetworkDeviceNotPresentCreatedWithUnusedName(c *gc.C) {
	s.patch()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")

	defaultBridge := &lxdapi.Network{
		Name:    network.DefaultLXDBridge,
		Type:    "bridge",
		Managed: true,
		NetworkPut: lxdapi.NetworkPut{
			Config: map[string]string{
				"ipv4.address": "auto",
				"ipv4.nat":     "true",
				"ipv6.address": "none",
				"ipv6.nat":     "false",
			},
		},
	}
	devReq := lxdapi.ProfilePut{
		Devices: map[string]map[string]string{
			"eth0": {},
			"eth1": {},
			// eth2 will be generated as the first unused device name.
			"eth2": {
				"parent":  network.DefaultLXDBridge,
				"type":    "nic",
				"nictype": "bridged",
			},
		},
	}
	gomock.InOrder(
		cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(defaultBridge, "", nil),
		cSvr.EXPECT().UpdateProfile("default", devReq, lxdtesting.ETag).Return(nil),
	)

	profile := defaultLegacyProfileWithNIC()
	profile.Devices["eth0"] = map[string]string{}
	profile.Devices["eth1"] = map[string]string{}

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(profile, lxdtesting.ETag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestVerifyNetworkDeviceNotPresentErrorForCluster(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerClustered(ctrl, "cluster-1")

	profile := defaultLegacyProfileWithNIC()
	delete(profile.Devices, "eth0")

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.VerifyNetworkDevice(profile, lxdtesting.ETag)
	c.Assert(err, gc.ErrorMatches, `profile "default" does not have any devices configured with type "nic"`)
}

func (s *networkSuite) TestInterfaceInfoFromDevices(c *gc.C) {
	nics := map[string]map[string]string{
		"eth0": {
			"parent":  network.DefaultLXDBridge,
			"type":    "nic",
			"nictype": "bridged",
			"hwaddr":  "00:16:3e:00:00:00",
		},
		"eno9": {
			"parent":  "br1",
			"type":    "nic",
			"nictype": "macvlan",
			"hwaddr":  "00:16:3e:00:00:3e",
		},
	}

	info, err := lxd.InterfaceInfoFromDevices(nics)
	c.Assert(err, jc.ErrorIsNil)

	exp := []corenetwork.InterfaceInfo{
		{
			InterfaceName:       "eno9",
			MACAddress:          "00:16:3e:00:00:3e",
			ConfigType:          corenetwork.ConfigDHCP,
			ParentInterfaceName: "br1",
		},
		{
			InterfaceName:       "eth0",
			MACAddress:          "00:16:3e:00:00:00",
			ConfigType:          corenetwork.ConfigDHCP,
			ParentInterfaceName: network.DefaultLXDBridge,
		},
	}
	c.Check(info, jc.DeepEquals, exp)
}

func (s *networkSuite) TestCheckAptLXDBridgeConfiguration(c *gc.C) {
	lxd.PatchLXDViaSnap(s, false)

	bridgeName, err := lxd.CheckBridgeConfigFile(validBridgeConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(bridgeName, gc.Equals, "lxdbr0")

	noBridge := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="false"
`), nil
	}
	_, err = lxd.CheckBridgeConfigFile(noBridge)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has USE_LXD_BRIDGE set to false
It looks like your LXD bridge has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	noSubnets := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="br0"
`), nil
	}
	_, err = lxd.CheckBridgeConfigFile(noSubnets)
	c.Assert(err.Error(), gc.Equals, `br0 has no ipv4 or ipv6 subnet enabled
It looks like your LXD bridge has not yet been configured. Configure it via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)

	ipv6 := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
LXD_IPV6_ADDR="2001:470:b368:4242::1"
`), nil
	}

	_, err = lxd.CheckBridgeConfigFile(ipv6)
	c.Assert(err.Error(), gc.Equals, lxd.BridgeConfigFile+` has IPv6 enabled.
Juju doesn't currently support IPv6.
Disable IPv6 via:

	sudo dpkg-reconfigure -p medium lxd

and run the command again.`)
}

func (s *networkSuite) TestCheckSnapLXDBridgeConfiguration(c *gc.C) {
	lxd.PatchLXDViaSnap(s, true)

	bridgeName, err := lxd.CheckBridgeConfigFile(validBridgeConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(bridgeName, gc.Equals, "lxdbr0")

	noBridge := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="false"
`), nil
	}
	_, err = lxd.CheckBridgeConfigFile(noBridge)
	c.Assert(err.Error(), gc.Equals, lxd.SnapBridgeConfigFile+` has USE_LXD_BRIDGE set to false
It looks like your LXD bridge has not yet been configured.`)

	noSubnets := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="br0"
`), nil
	}
	_, err = lxd.CheckBridgeConfigFile(noSubnets)
	c.Assert(err.Error(), gc.Equals, `br0 has no ipv4 or ipv6 subnet enabled
It looks like your LXD bridge has not yet been configured.`)

	ipv6 := func(string) ([]byte, error) {
		return []byte(`
USE_LXD_BRIDGE="true"
LXD_BRIDGE="lxdbr0"
LXD_IPV6_ADDR="2001:470:b368:4242::1"
`), nil
	}

	_, err = lxd.CheckBridgeConfigFile(ipv6)
	c.Assert(err.Error(), gc.Equals, lxd.SnapBridgeConfigFile+` has IPv6 enabled.
Juju doesn't currently support IPv6.`)
}

func (s *networkSuite) TestVerifyNICsWithConfigFileNICFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = lxd.VerifyNICsWithConfigFile(jujuSvr, defaultLegacyProfileWithNIC().Devices, validBridgeConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(jujuSvr.LocalBridgeName(), gc.Equals, "lxdbr0")
}

func (s *networkSuite) TestVerifyNICsWithConfigFileNICNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	nics := defaultLegacyProfileWithNIC().Devices
	nics["eth0"]["parent"] = "br0"

	err = lxd.VerifyNICsWithConfigFile(jujuSvr, nics, validBridgeConfig)
	c.Assert(err, gc.ErrorMatches,
		`no network device found with nictype "bridged" or "macvlan" that uses the configured bridge in `+
			lxd.BridgeConfigFile+"\n\tthe following devices were checked: "+`\[eth0\]`)
}

func validBridgeConfig(_ string) ([]byte, error) {
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

func (s *networkSuite) TestEnableHTTPSListener(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := &lxdapi.Server{}
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil).Times(2),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "[::]",
			},
		}, lxdtesting.ETag).Return(nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.EnableHTTPSListener()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestEnableHTTPSListenerWithFallbackToIPv4(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := &lxdapi.Server{}
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil).Times(2),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "[::]",
			},
		}, lxdtesting.ETag).Return(errors.New(lxd.ErrIPV6NotSupported)),
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "0.0.0.0",
			},
		}, lxdtesting.ETag).Return(nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.EnableHTTPSListener()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkSuite) TestEnableHTTPSListenerWithErrors(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := &lxdapi.Server{}
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	// check on the first request
	cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, errors.New("bad"))

	err = jujuSvr.EnableHTTPSListener()
	c.Assert(err, gc.ErrorMatches, "bad")

	// check on the second request
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "[::]",
			},
		}, lxdtesting.ETag).Return(errors.New(lxd.ErrIPV6NotSupported)),
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, errors.New("bad")),
	)

	err = jujuSvr.EnableHTTPSListener()
	c.Assert(err, gc.ErrorMatches, "bad")

	// check on the third request
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "[::]",
			},
		}, lxdtesting.ETag).Return(errors.New(lxd.ErrIPV6NotSupported)),
		cSvr.EXPECT().GetServer().Return(cfg, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateServer(lxdapi.ServerPut{
			Config: map[string]interface{}{
				"core.https_address": "0.0.0.0",
			},
		}, lxdtesting.ETag).Return(errors.New("bad")),
	)

	err = jujuSvr.EnableHTTPSListener()
	c.Assert(err, gc.ErrorMatches, "bad")
}

func (s *networkSuite) TestNewNICDeviceWithoutMACAddressOrMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "", 0)
	expected := map[string]string{
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (s *networkSuite) TestNewNICDeviceWithMACAddressButNoMTU(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 0)
	expected := map[string]string{
		"hwaddr":  "aa:bb:cc:dd:ee:f0",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (s *networkSuite) TestNewNICDeviceWithoutMACAddressButMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "", 1492)
	expected := map[string]string{
		"mtu":     "1492",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (s *networkSuite) TestNewNICDeviceWithMACAddressAndMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 9000)
	expected := map[string]string{
		"hwaddr":  "aa:bb:cc:dd:ee:f0",
		"mtu":     "9000",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}
