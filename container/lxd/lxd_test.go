// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"runtime"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
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
			t.Skip("LXD is not avalilable %s", err)
		}
	*/
	gc.TestingT(t)
}

type LxdSuite struct{}

var _ = gc.Suite(&LxdSuite{})

func (t *LxdSuite) makeManager(c *gc.C, name string) container.Manager {
	config := container.ManagerConfig{
		container.ConfigModelUUID: testing.ModelTag.Id(),
	}

	manager, err := lxd.NewContainerManager(config)
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
	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.Config = envConfig
	storageConfig := &container.StorageConfig{}
	networkConfig := container.BridgeNetworkConfig("nic42", 4321, nil)

	manager := t.makeManager(c, "manager")
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error { return nil }
	_, _, err = manager.CreateContainer(
		instanceConfig,
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

func (t *LxdSuite) TestNICDeviceWithInvalidDeviceName(c *gc.C) {
	device, err := lxd.NICDevice("", "br-eth1", "", 0)
	c.Assert(device, gc.IsNil)
	c.Assert(err.Error(), gc.Equals, "invalid device name")
}

func (t *LxdSuite) TestNICDeviceWithInvalidParentDeviceName(c *gc.C) {
	device, err := lxd.NICDevice("eth0", "", "", 0)
	c.Assert(device, gc.IsNil)
	c.Assert(err.Error(), gc.Equals, "invalid parent device name")
}

func (t *LxdSuite) TestNICDeviceWithoutMACAddressOrMTUGreaterThanZero(c *gc.C) {
	device, err := lxd.NICDevice("eth1", "br-eth1", "", 0)
	c.Assert(err, gc.IsNil)
	expected := lxdclient.Device{
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNICDeviceWithMACAddressButNoMTU(c *gc.C) {
	device, err := lxd.NICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 0)
	c.Assert(err, gc.IsNil)
	expected := lxdclient.Device{
		"hwaddr":  "aa:bb:cc:dd:ee:f0",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNICDeviceWithoutMACAddressButMTUGreaterThanZero(c *gc.C) {
	device, err := lxd.NICDevice("eth1", "br-eth1", "", 1492)
	c.Assert(err, gc.IsNil)
	expected := lxdclient.Device{
		"mtu":     "1492",
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNICDeviceWithMACAddressAndMTUGreaterThanZero(c *gc.C) {
	device, err := lxd.NICDevice("eth1", "br-eth1", "aa:bb:cc:dd:ee:f0", 9000)
	c.Assert(err, gc.IsNil)
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

func (t *LxdSuite) TestNetworkDevicesWithEmptyParentDevice(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		Address:             network.NewAddress("0.10.0.20"),
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}, {
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		Address:       network.NewAddress("0.10.0.21"),
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           9000,
	}}

	expected := lxdclient.Devices{
		"eth0": lxdclient.Device{
			"hwaddr":  "aa:bb:cc:dd:ee:f0",
			"name":    "eth0",
			"nictype": "bridged",
			"parent":  "br-eth0",
			"type":    "nic",
		},
		"eth1": lxdclient.Device{
			"hwaddr":  "aa:bb:cc:dd:ee:f1",
			"name":    "eth1",
			"nictype": "bridged",
			"parent":  "lxdbr0",
			"type":    "nic",
			"mtu":     "9000",
		},
	}

	result, err := lxd.NetworkDevices(&container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}
