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
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"gopkg.in/juju/charm.v6"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type managerSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&managerSuite{})

func (s *managerSuite) patch(svr lxdclient.ImageServer) {
	lxd.PatchConnectRemote(s, map[string]lxdclient.ImageServer{"cloud-images.ubuntu.com": svr})
	lxd.PatchGenerateVirtualMACAddress(s)
}

func (s *managerSuite) makeManager(c *gc.C, svr lxdclient.ContainerServer) container.Manager {
	return s.makeManagerForConfig(c, getBaseConfig(), svr)
}

func (s *managerSuite) makeManagerForConfig(
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

func (s *managerSuite) TestContainerCreateDestroy(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	s.patch(cSvr)

	manager := s.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostName, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	// Operation arrangements.
	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	exp := cSvr.EXPECT()

	// Arrangements for the container creation.
	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+s.Arch(), "foo-target")
	exp.UpdateContainerState(hostName, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(startOp, nil)

	exp.GetContainerState(hostName).Return(
		&lxdapi.ContainerState{StatusCode: lxdapi.Running}, lxdtesting.ETag, nil).Times(2)

	exp.GetContainer(hostName).Return(&lxdapi.Container{Name: hostName}, lxdtesting.ETag, nil)

	// Arrangements for the container destruction.
	stopReq := lxdapi.ContainerStatePut{
		Action:   "stop",
		Timeout:  -1,
		Stateful: false,
		Force:    true,
	}
	gomock.InOrder(
		exp.UpdateContainerState(hostName, stopReq, lxdtesting.ETag).Return(stopOp, nil),
		exp.DeleteContainer(hostName).Return(deleteOp, nil),
	)

	instance, hc, err := manager.CreateContainer(
		iCfg, constraints.Value{}, "xenial", prepNetworkConfig(), &container.StorageConfig{}, lxdtesting.NoOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)

	instanceId := instance.Id()
	c.Check(string(instanceId), gc.Equals, hostName)

	instanceStatus := instance.Status(context.NewCloudCallContext())
	c.Check(instanceStatus.Status, gc.Equals, status.Running)
	c.Check(*hc.AvailabilityZone, gc.Equals, "test-availability-zone")

	err = manager.DestroyContainer(instanceId)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *managerSuite) TestContainerCreateUpdateIPv4Network(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServerWithExtensions(ctrl, "network")
	s.patch(cSvr)

	manager := s.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostName, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	exp := cSvr.EXPECT()

	req := lxdapi.NetworkPut{
		Config: map[string]string{
			"ipv4.address": "auto",
			"ipv4.nat":     "true",
		},
	}
	gomock.InOrder(
		exp.GetNetwork(network.DefaultLXDBridge).Return(&lxdapi.Network{}, lxdtesting.ETag, nil),
		exp.UpdateNetwork(network.DefaultLXDBridge, req, lxdtesting.ETag).Return(nil),
	)

	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+s.Arch(), "foo-target")

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	exp.UpdateContainerState(hostName, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(startOp, nil)
	exp.GetContainer(hostName).Return(&lxdapi.Container{Name: hostName}, lxdtesting.ETag, nil)

	// Supplying config for a single device with default bridge and without a
	// CIDR will cause the default bridge to be updated with IPv4 config.
	netConfig := container.BridgeNetworkConfig("eth0", 1500, []network.InterfaceInfo{{
		InterfaceName:       "eth0",
		InterfaceType:       network.EthernetInterface,
		ConfigType:          network.ConfigDHCP,
		ParentInterfaceName: network.DefaultLXDBridge,
	}})
	_, _, err = manager.CreateContainer(
		iCfg, constraints.Value{}, "xenial", netConfig, &container.StorageConfig{}, lxdtesting.NoOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *managerSuite) TestCreateContainerCreateFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	createRemoteOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createRemoteOp.EXPECT().Wait().Return(nil).AnyTimes()
	createRemoteOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Failure, Err: "create failed"}, nil)

	exp := cSvr.EXPECT()

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		exp.GetImageAlias("juju/xenial/"+s.Arch()).Return(alias, lxdtesting.ETag, nil),
		exp.GetImage("foo-target").Return(&image, lxdtesting.ETag, nil),
		exp.CreateContainerFromImage(cSvr, image, gomock.Any()).Return(createRemoteOp, nil),
	)

	_, _, err := s.makeManager(c, cSvr).CreateContainer(
		prepInstanceConfig(c),
		constraints.Value{},
		"xenial",
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*create failed")
}

func (s *managerSuite) TestCreateContainerSpecCreationError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	// When the local image acquisition fails, this will cause the remote
	// connection attempt to fail.
	// This is our error condition exit from manager.getContainerSpec.
	lxd.PatchConnectRemote(s, map[string]lxdclient.ImageServer{})

	exp := cSvr.EXPECT()

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		exp.GetImageAlias("juju/xenial/"+s.Arch()).Return(alias, lxdtesting.ETag, nil),
		exp.GetImage("foo-target").Return(&image, lxdtesting.ETag, errors.New("not here")),
	)

	_, _, err := s.makeManager(c, cSvr).CreateContainer(
		prepInstanceConfig(c),
		constraints.Value{},
		"xenial",
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*unrecognized remote server")
}

func (s *managerSuite) TestCreateContainerStartFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	s.patch(cSvr)

	manager := s.makeManager(c, cSvr)
	iCfg := prepInstanceConfig(c)
	hostName, err := manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	updateOp := lxdtesting.NewMockOperation(ctrl)
	updateOp.EXPECT().Wait().Return(errors.New("start failed"))

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil).AnyTimes()

	exp := cSvr.EXPECT()

	expectCreateContainer(ctrl, cSvr, "juju/xenial/"+s.Arch(), "foo-target")
	gomock.InOrder(
		exp.UpdateContainerState(
			hostName, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(updateOp, nil),
		exp.GetContainerState(hostName).Return(&lxdapi.ContainerState{StatusCode: lxdapi.Stopped}, lxdtesting.ETag, nil),
		exp.DeleteContainer(hostName).Return(deleteOp, nil),
	)

	_, _, err = manager.CreateContainer(
		iCfg,
		constraints.Value{},
		"xenial",
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
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

	exp := svr.EXPECT()

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: target}}
	exp.GetImageAlias(aliasName).Return(alias, lxdtesting.ETag, nil)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	exp.GetImage("foo-target").Return(&image, lxdtesting.ETag, nil)
	exp.CreateContainerFromImage(svr, image, gomock.Any()).Return(createRemoteOp, nil)
}

func (s *managerSuite) TestListContainers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	manager := s.makeManager(c, cSvr)

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

func (s *managerSuite) TestIsInitialized(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	manager := s.makeManager(c, cSvr)
	c.Check(manager.IsInitialized(), gc.Equals, true)
}

func (s *managerSuite) TestNetworkDevicesFromConfigWithEmptyParentDevice(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	interfaces := []network.InterfaceInfo{{
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           9000,
	}}

	result, _, err := lxd.NetworkDevicesFromConfig(s.makeManager(c, cSvr), &container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, gc.ErrorMatches, "parent interface name is empty")
	c.Assert(result, gc.IsNil)
}

func (s *managerSuite) TestNetworkDevicesFromConfigWithParentDevice(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

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

	result, unknown, err := lxd.NetworkDevicesFromConfig(s.makeManager(c, cSvr), &container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
	c.Check(unknown, gc.HasLen, 0)
}

func (s *managerSuite) TestNetworkDevicesFromConfigUnknownCIDR(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	interfaces := []network.InterfaceInfo{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}}

	_, unknown, err := lxd.NetworkDevicesFromConfig(s.makeManager(c, cSvr), &container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(unknown, gc.DeepEquals, []string{"br-eth0"})
}

func (s *managerSuite) TestNetworkDevicesFromConfigNoInputGetsProfileNICs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	s.patch(cSvr)

	cSvr.EXPECT().GetProfile("default").Return(defaultProfileWithNIC(), lxdtesting.ETag, nil)

	result, _, err := lxd.NetworkDevicesFromConfig(s.makeManager(c, cSvr), &container.NetworkConfig{})
	c.Assert(err, jc.ErrorIsNil)

	exp := map[string]map[string]string{
		"eth0": {
			"parent":  network.DefaultLXDBridge,
			"type":    "nic",
			"nictype": "bridged",
			"hwaddr":  "00:16:3e:00:00:00",
		},
	}

	c.Check(result, gc.DeepEquals, exp)
}

func (s *managerSuite) TestGetImageSourcesDefaultConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	mgr := s.makeManager(c, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (s *managerSuite) TestGetImageSourcesNonStandardStreamDefaultConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "nope"
	mgr := s.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (s *managerSuite) TestGetImageSourcesDailyOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := s.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesDailyRemote})
}

func (s *managerSuite) TestGetImageSourcesImageMetadataURLExpectedHTTPSSources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	mgr := s.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.ServerSpec{
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

func (s *managerSuite) TestGetImageSourcesImageMetadataURLDailyStream(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := s.makeManagerForConfig(c, cfg, cSvr)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.ServerSpec{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
		lxd.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (s *managerSuite) TestMaybeWriteLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	mgr := s.makeManager(c, cSvr)
	proMgr, ok := mgr.(container.LXDProfileManager)
	c.Assert(ok, jc.IsTrue)

	put := charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
		Description: "lxd profile for testing",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
				"type": "unix-char",
			},
		},
	}
	post := lxdapi.ProfilesPost{
		ProfilePut: lxdapi.ProfilePut(put),
		Name:       "juju-default-lxd-0",
	}
	cSvr.EXPECT().CreateProfile(post).Return(nil).Times(1)
	cSvr.EXPECT().GetProfileNames().Return([]string{"default", "custom"}, nil).Times(1)

	err := proMgr.MaybeWriteLXDProfile("juju-default-lxd-0", &put)
	c.Assert(err, jc.ErrorIsNil)
}
