// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"runtime"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdclient"
)

func Test(t *stdtesting.T) {
	if runtime.GOOS == "windows" {
		t.Skip("LXC is not supported on windows")
	}

	/* if there's not a lxd available, don't run the tests */
	/*
		_, err := lxd.ConnectLocal("")
		if err != nil {
			t.Skip("LXD is not available %s", err)
		}
	*/
	gc.TestingT(t)
}

type LxdSuite struct{}

var _ = gc.Suite(&LxdSuite{})

func (t *LxdSuite) baseConfig() container.ManagerConfig {
	return container.ManagerConfig{
		container.ConfigModelUUID: testing.ModelTag.Id(),
	}
}

func (t *LxdSuite) makeManager(c *gc.C, conf container.ManagerConfig) container.Manager {
	manager, err := lxd.NewContainerManager(conf)
	c.Assert(err, jc.ErrorIsNil)
	return manager
}

func (t *LxdSuite) TestNotAllContainersAreDeleted(c *gc.C) {
	c.Skip("Test skipped because it talks directly to LXD agent.")
	lxdClient, err := lxd.ConnectLocal()
	c.Assert(err, jc.ErrorIsNil)

	/* create a container to make sure isn't deleted */
	instanceSpec := lxdclient.InstanceSpec{
		Name:  "juju-lxd-tests",
		Image: "ubuntu-xenial",
	}

	_, err = lxdClient.AddInstance(instanceSpec)
	c.Assert(err, jc.ErrorIsNil)
	defer lxdClient.RemoveInstances("", "juju-lxd-tests")

	instanceConfig, err := containertesting.MockMachineConfig("1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	storageConfig := &container.StorageConfig{}
	networkConfig := container.BridgeNetworkConfig("nic42", 4321, nil)

	manager := t.makeManager(c, t.baseConfig())
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error { return nil }
	_, _, err = manager.CreateContainer(
		instanceConfig,
		constraints.Value{},
		"xenial",
		networkConfig,
		storageConfig,
		callback,
	)
	c.Assert(err, jc.ErrorIsNil)

	instances, err := manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)

	for _, inst := range instances {
		err = manager.DestroyContainer(inst.Id())
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (t *LxdSuite) TestNewNICDeviceWithoutMACAddressOrMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "", 0)
	expected := lxdclient.Device{
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNewNICDeviceWithMACAddressButNoMTU(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 0)
	expected := lxdclient.Device{
		"hwaddr":  "aa:bb:cc:dd:ee:f0",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNewNICDeviceWithoutMACAddressButMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "", 1492)
	expected := lxdclient.Device{
		"mtu":     "1492",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNewNICDeviceWithMACAddressAndMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 9000)
	expected := lxdclient.Device{
		"hwaddr":  "aa:bb:cc:dd:ee:f0",
		"mtu":     "9000",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNetworkDevicesFromConfigWithEmptyParentDevice(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           9000,
	}}

	result, _, err := lxd.NetworkDevicesFromConfig(&container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, gc.ErrorMatches, "parent interface name is empty")
	c.Assert(result, gc.IsNil)
}

func (t *LxdSuite) TestNetworkDevicesFromConfigWithParentDevice(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		CIDR:                "10.10.0.0/24",
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}}

	expected := lxdclient.Devices{
		"eth0": {
			"hwaddr":  "aa:bb:cc:dd:ee:f0",
			"name":    "eth0",
			"nictype": "bridged",
			"parent":  "br-eth0",
			"type":    "nic",
		},
	}

	result, unknown, err := lxd.NetworkDevicesFromConfig(&container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
	c.Check(unknown, gc.HasLen, 0)
}

func (t *LxdSuite) TestNetworkDevicesFromConfigUnknownCIDR(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}}

	_, unknown, err := lxd.NetworkDevicesFromConfig(&container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(unknown, gc.DeepEquals, []string{"br-eth0"})
}

func (t *LxdSuite) TestGetImageSourcesDefaultConfig(c *gc.C) {
	mgr := t.makeManager(c, t.baseConfig())

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdclient.Remote{lxdclient.CloudImagesRemote, lxdclient.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesNonStandardStreamDefaultConfig(c *gc.C) {
	cfg := t.baseConfig()
	cfg[config.ContainerImageStreamKey] = "nope"
	mgr := t.makeManager(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdclient.Remote{lxdclient.CloudImagesRemote, lxdclient.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesDailyOnly(c *gc.C) {
	cfg := t.baseConfig()
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManager(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdclient.Remote{lxdclient.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLExpectedHTTPSSources(c *gc.C) {
	cfg := t.baseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	mgr := t.makeManager(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxdclient.Remote{
		{
			Name:          "special.container.sauce",
			Host:          "https://special.container.sauce",
			Protocol:      lxdclient.SimplestreamsProtocol,
			Cert:          nil,
			ServerPEMCert: "",
		},
		lxdclient.CloudImagesRemote,
		lxdclient.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLDailyStream(c *gc.C) {
	cfg := t.baseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManager(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxdclient.Remote{
		{
			Name:          "special.container.sauce",
			Host:          "https://special.container.sauce",
			Protocol:      lxdclient.SimplestreamsProtocol,
			Cert:          nil,
			ServerPEMCert: "",
		},
		lxdclient.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}
