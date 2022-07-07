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
		NetworkPut: lxdapi.NetworkPut{Config: netConf},
	}
	newNet := &lxdapi.Network{
		Name:       network.DefaultLXDBridge,
		Type:       "bridge",
		Managed:    true,
		NetworkPut: lxdapi.NetworkPut{Config: netConf},
	}
	gomock.InOrder(
		cSvr.EXPECT().GetNetwork(network.DefaultLXDBridge).Return(nil, "", errors.New("network not found")),
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

func (s *networkSuite) TestVerifyNetworkDeviceNotPresentNoNetAPIError(c *gc.C) {
	s.patch()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	profile := defaultLegacyProfileWithNIC()
	delete(profile.Devices, "eth0")

	err = jujuSvr.VerifyNetworkDevice(profile, lxdtesting.ETag)
	c.Assert(err, gc.ErrorMatches, `profile "default" does not have any devices configured with type "nic"`)
}

func (s *networkSuite) TestVerifyNetworkDevicePresentNoNetAPIError(c *gc.C) {
	s.patch()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	profile := defaultLegacyProfileWithNIC()

	err = jujuSvr.VerifyNetworkDevice(profile, lxdtesting.ETag)
	c.Assert(err, gc.ErrorMatches, "versions of LXD without network API not supported")
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

	exp := corenetwork.InterfaceInfos{
		{
			InterfaceName:       "eno9",
			MACAddress:          "00:16:3e:00:00:3e",
			ConfigType:          corenetwork.ConfigDHCP,
			ParentInterfaceName: "br1",
			Origin:              corenetwork.OriginProvider,
		},
		{
			InterfaceName:       "eth0",
			MACAddress:          "00:16:3e:00:00:00",
			ConfigType:          corenetwork.ConfigDHCP,
			ParentInterfaceName: network.DefaultLXDBridge,
			Origin:              corenetwork.OriginProvider,
		},
	}
	c.Check(info, jc.DeepEquals, exp)
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
