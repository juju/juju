// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"
	stdtesting "testing"

	"github.com/golang/mock/gomock"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	lxdclient "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type LxdSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&LxdSuite{})

func (t *LxdSuite) patch(svr lxdclient.ImageServer) {
	lxd.PatchConnectRemote(t, map[string]lxdclient.ImageServer{"cloud-images.ubuntu.com": svr})
}

func (t *LxdSuite) makeManager(c *gc.C, svr lxdclient.ContainerServer) container.Manager {
	return t.makeManagerForConfig(c, getBaseConfig(), svr)
}

func (t *LxdSuite) makeManagerForConfig(
	c *gc.C, cfg container.ManagerConfig, cSvr lxdclient.ContainerServer,
) container.Manager {
	svr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	manager, err := lxd.NewContainerManager(cfg, svr)
	c.Assert(err, jc.ErrorIsNil)
	return manager
}

func getBaseConfig() container.ManagerConfig {
	return container.ManagerConfig{
		container.ConfigModelUUID:        coretesting.ModelTag.Id(),
		container.ConfigAvailabilityZone: "test-availability-zone",
		config.ContainerImageStreamKey:   "released",
	}
}

func prepInstanceConfig(c *gc.C) *instancecfg.InstanceConfig {
	apiInfo := &api.Info{
		Addrs:    []string{"127.0.0.1:1337"},
		Password: "password",
		Nonce:    "nonce",
		Tag:      names.NewMachineTag("123"),
		ModelTag: names.NewModelTag("3fe11b2c-ae46-4cd1-96ad-d34239f70daf"),
		CACert:   "foo",
	}
	icfg, err := instancecfg.NewInstanceConfig(
		names.NewControllerTag("4e29484b-4ff3-417f-97c3-09d1bec98d5b"),
		"123",
		"nonce",
		"imagestream",
		"xenial",
		apiInfo,
	)
	c.Assert(err, jc.ErrorIsNil)
	instancecfg.PopulateInstanceConfig(
		icfg,
		"lxd",
		"",
		false,
		proxy.Settings{},
		proxy.Settings{},
		proxy.Settings{},
		"",
		false,
		false,
		nil,
	)
	list := coretools.List{
		&coretools.Tools{Version: version.MustParseBinary("2.3.4-trusty-amd64")},
	}
	err = icfg.SetTools(list)
	c.Assert(err, jc.ErrorIsNil)
	return icfg
}

func prepNetworkConfig() *container.NetworkConfig {
	return container.BridgeNetworkConfig("eth0", 1500, []network.InterfaceInfo{{
		InterfaceName:       "eth0",
		InterfaceType:       network.EthernetInterface,
		ConfigType:          network.ConfigDHCP,
		ParentInterfaceName: "eth0",
	}})
}

var noOpCallback = func(settableStatus status.Status, info string, data map[string]interface{}) error { return nil }

func (t *LxdSuite) TestContainerCreateDestroy(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)
	t.patch(cSvr)

	manager := t.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostname, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	// Operation arrangements.
	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	// Arrangements for the container creation.
	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+t.Arch(), "foo-target")
	cSvr.EXPECT().UpdateContainerState(
		hostname, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(startOp, nil)

	cSvr.EXPECT().GetContainerState(hostname).Return(
		&lxdapi.ContainerState{StatusCode: lxdapi.Running}, lxdtesting.ETag, nil).Times(2)

	// Arrangements for the container destruction.
	gomock.InOrder(
		cSvr.EXPECT().UpdateContainerState(
			hostname, lxdapi.ContainerStatePut{Action: "stop", Timeout: -1}, lxdtesting.ETag).Return(stopOp, nil),
		cSvr.EXPECT().DeleteContainer(hostname).Return(deleteOp, nil),
	)

	instance, hc, err := manager.CreateContainer(
		iCfg, constraints.Value{}, "xenial", prepNetworkConfig(), &container.StorageConfig{}, noOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)

	instanceId := instance.Id()
	c.Check(string(instanceId), gc.Equals, hostname)

	instanceStatus := instance.Status(context.NewCloudCallContext())
	c.Check(instanceStatus.Status, gc.Equals, status.Running)
	c.Check(*hc.AvailabilityZone, gc.Equals, "test-availability-zone")

	err = manager.DestroyContainer(instanceId)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *LxdSuite) TestContainerCreateUpdateIPv4Network(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServerWithExtensions(ctrl, "network")
	t.patch(cSvr)

	manager := t.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostname, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

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

	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+t.Arch(), "foo-target")

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)
	cSvr.EXPECT().UpdateContainerState(
		hostname, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(startOp, nil)

	// Supplying config for a single device with default bridge and without a
	// CIDR will cause the default bridge to be updated with IPv4 config.
	netConfig := container.BridgeNetworkConfig("eth0", 1500, []network.InterfaceInfo{{
		InterfaceName:       "eth0",
		InterfaceType:       network.EthernetInterface,
		ConfigType:          network.ConfigDHCP,
		ParentInterfaceName: network.DefaultLXDBridge,
	}})
	_, _, err = manager.CreateContainer(
		iCfg, constraints.Value{}, "xenial", netConfig, &container.StorageConfig{}, noOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *LxdSuite) TestCreateContainerCreateFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	createRemoteOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createRemoteOp.EXPECT().Wait().Return(nil).AnyTimes()
	createRemoteOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Failure, Err: "create failed"}, nil)

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		cSvr.EXPECT().GetImageAlias("juju/xenial/"+t.Arch()).Return(alias, lxdtesting.ETag, nil),
		cSvr.EXPECT().GetImage("foo-target").Return(&image, lxdtesting.ETag, nil),
		cSvr.EXPECT().CreateContainerFromImage(cSvr, image, gomock.Any()).Return(createRemoteOp, nil),
	)

	_, _, err := t.makeManager(c, cSvr).CreateContainer(
		prepInstanceConfig(c),
		constraints.Value{},
		"xenial",
		prepNetworkConfig(),
		&container.StorageConfig{},
		noOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*create failed")
}

func (t *LxdSuite) TestCreateContainerStartFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)
	t.patch(cSvr)

	manager := t.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostname, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	updateOp := lxdtesting.NewMockOperation(ctrl)
	updateOp.EXPECT().Wait().Return(errors.New("start failed"))

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil).AnyTimes()

	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+t.Arch(), "foo-target")
	gomock.InOrder(
		cSvr.EXPECT().UpdateContainerState(
			hostname, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(updateOp, nil),
		cSvr.EXPECT().DeleteContainer(hostname).Return(deleteOp, nil),
	)

	_, _, err = manager.CreateContainer(
		iCfg,
		constraints.Value{},
		"xenial",
		prepNetworkConfig(),
		&container.StorageConfig{},
		noOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*start failed")
}

// expectCreateContainer is a convenience function for the expectations
// concerning a successful container creation based on a cached local
// image.
func expectCreateContainer(ctrl *gomock.Controller, svr *lxdtesting.MockContainerServer, aliasName, target string) {
	createRemoteOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createRemoteOp.EXPECT().Wait().Return(nil).AnyTimes()
	createRemoteOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: target}}
	svr.EXPECT().GetImageAlias(aliasName).Return(alias, lxdtesting.ETag, nil)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	svr.EXPECT().GetImage("foo-target").Return(&image, lxdtesting.ETag, nil)
	svr.EXPECT().CreateContainerFromImage(svr, image, gomock.Any()).Return(createRemoteOp, nil)
}

func (t *LxdSuite) TestListContainers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)
	manager := t.makeManager(c, cSvr)

	prefix := manager.Namespace().Prefix()
	wrongPrefix := prefix[:len(prefix)-1] + "j"

	containers := []lxdapi.Container{
		{Name: "foobar"},
		{Name: "definitely-not-a-juju-container"},
		{Name: wrongPrefix + "-0"},
		{Name: prefix + "-0"},
		{Name: "please-disperse"},
		{Name: prefix + "-1"},
		{Name: "nothing-to-see-here-please"},
	}

	cSvr.EXPECT().GetContainers().Return(containers, nil)

	result, err := manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 2)
	c.Check(string(result[0].Id()), gc.Equals, prefix+"-0")
	c.Check(string(result[1].Id()), gc.Equals, prefix+"-1")
}

func (t *LxdSuite) TestIsInitialized(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	manager := t.makeManager(c, cSvr)
	c.Check(manager.IsInitialized(), gc.Equals, true)
}

func (t *LxdSuite) TestNewNICDeviceWithoutMACAddressOrMTUGreaterThanZero(c *gc.C) {
	device := lxd.NewNICDevice("eth1", "br-eth1", "", 0)
	expected := map[string]string{
		"name":    "eth1",
		"nictype": "bridged",
		"parent":  "br-eth1",
		"type":    "nic",
	}
	c.Assert(device, gc.DeepEquals, expected)
}

func (t *LxdSuite) TestNewNICDeviceWithMACAddressButNoMTU(c *gc.C) {
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

func (t *LxdSuite) TestNewNICDeviceWithoutMACAddressButMTUGreaterThanZero(c *gc.C) {
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

func (t *LxdSuite) TestNewNICDeviceWithMACAddressAndMTUGreaterThanZero(c *gc.C) {
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

	expected := map[string]map[string]string{
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	mgr := t.makeManager(c, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.RemoteServer{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesNonStandardStreamDefaultConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "nope"
	mgr := t.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.RemoteServer{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesDailyOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.RemoteServer{lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLExpectedHTTPSSources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	mgr := t.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.RemoteServer{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
		lxd.CloudImagesRemote,
		lxd.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLDailyStream(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := t.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.RemoteServer{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
		lxd.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}
