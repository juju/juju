// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"
	"errors"

	lxdclient "github.com/canonical/lxd/client"
	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/lxd"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
	"github.com/juju/juju/internal/network"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type managerSuite struct {
	lxdtesting.BaseSuite

	cSvr           *lxdtesting.MockInstanceServer
	createRemoteOp *lxdtesting.MockRemoteOperation
	deleteOp       *lxdtesting.MockOperation
	startOp        *lxdtesting.MockOperation
	stopOp         *lxdtesting.MockOperation
	updateOp       *lxdtesting.MockOperation
	manager        container.Manager
}

var _ = gc.Suite(&managerSuite{})

func (s *managerSuite) patch() {
	lxd.PatchConnectRemote(s, map[string]lxdclient.ImageServer{"cloud-images.ubuntu.com": s.cSvr})
	lxd.PatchGenerateVirtualMACAddress(s)
}

func (s *managerSuite) makeManager(c *gc.C) {
	s.makeManagerForConfig(c, getBaseConfig())
}

func (s *managerSuite) makeManagerForConfig(c *gc.C, cfg container.ManagerConfig) {
	manager, err := lxd.NewContainerManager(cfg, func() (*lxd.Server, error) { return lxd.NewServer(s.cSvr) })
	c.Assert(err, jc.ErrorIsNil)
	s.manager = manager
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
		corebase.MakeDefaultBase("ubuntu", "16.04"),
		apiInfo,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = instancecfg.PopulateInstanceConfig(
		icfg,
		"lxd",
		false,
		instancecfg.ProxyConfiguration{},
		false,
		false,
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	list := coretools.List{
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64")},
	}
	err = icfg.SetTools(list)
	c.Assert(err, jc.ErrorIsNil)
	return icfg
}

func prepNetworkConfig() *container.NetworkConfig {
	return container.BridgeNetworkConfig(1500, corenetwork.InterfaceInfos{{
		InterfaceName:       "eth0",
		InterfaceType:       corenetwork.EthernetDevice,
		ConfigType:          corenetwork.ConfigDHCP,
		ParentInterfaceName: "eth0",
	}})
}

func (s *managerSuite) TestContainerCreateDestroy(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.patch()
	s.makeManager(c)

	iCfg := prepInstanceConfig(c)
	hostName, err := s.manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	// Operation arrangements.
	s.expectStartOp(ctrl)
	s.expectStopOp(ctrl)
	s.expectDeleteOp(ctrl)

	exp := s.cSvr.EXPECT()

	// Arrangements for the container creation.
	s.expectCreateContainer(ctrl)
	exp.UpdateInstanceState(hostName, lxdapi.InstanceStatePut{Action: "start", Timeout: -1}, "").Return(s.startOp, nil)

	exp.GetInstanceState(hostName).Return(
		&lxdapi.InstanceState{

			StatusCode: lxdapi.Running,
			Network: map[string]lxdapi.InstanceStateNetwork{
				"fan0": {
					Type: "fan",
				},
				"eth0": {
					HostName: "1lxd2-0",
					Type:     "bridged",
				},
			},
		}, lxdtesting.ETag, nil).Times(2)
	inst := &lxdapi.Instance{
		Name:         hostName,
		Type:         "container",
		Architecture: "amd64",
	}
	exp.GetInstance(hostName).Return(inst, lxdtesting.ETag, nil)

	// Arrangements for the container destruction.
	stopReq := lxdapi.InstanceStatePut{
		Action:   "stop",
		Timeout:  -1,
		Stateful: false,
		Force:    true,
	}
	gomock.InOrder(
		exp.UpdateInstanceState(hostName, stopReq, lxdtesting.ETag).Return(s.stopOp, nil),
		exp.DeleteInstance(hostName).Return(s.deleteOp, nil),
	)

	instance, hc, err := s.manager.CreateContainer(
		context.Background(), iCfg, constraints.Value{}, corebase.MakeDefaultBase("ubuntu", "16.04"), prepNetworkConfig(), &container.StorageConfig{}, lxdtesting.NoOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)

	instanceId := instance.Id()
	c.Check(string(instanceId), gc.Equals, hostName)
	c.Check(hc.Arch, gc.NotNil)
	c.Check(*hc.Arch, gc.Equals, "amd64")

	instanceStatus := instance.Status(context.Background())
	c.Check(instanceStatus.Status, gc.Equals, status.Running)
	c.Check(*hc.AvailabilityZone, gc.Equals, "test-availability-zone")

	err = s.manager.DestroyContainer(instanceId)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *managerSuite) TestContainerCreateUpdateIPv4Network(c *gc.C) {
	ctrl := s.setupWithExtensions(c, "network")
	defer ctrl.Finish()

	s.patch()

	s.makeManager(c)
	iCfg := prepInstanceConfig(c)
	hostName, err := s.manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	exp := s.cSvr.EXPECT()

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

	s.expectCreateContainer(ctrl)
	s.expectStartOp(ctrl)

	exp.UpdateInstanceState(hostName, lxdapi.InstanceStatePut{Action: "start", Timeout: -1}, "").Return(s.startOp, nil)
	inst := &lxdapi.Instance{
		Name:         hostName,
		Type:         "container",
		Architecture: "amd64",
	}
	exp.GetInstance(hostName).Return(inst, lxdtesting.ETag, nil)

	// Supplying config for a single device with default bridge and without a
	// CIDR will cause the default bridge to be updated with IPv4 config.
	netConfig := container.BridgeNetworkConfig(1500, corenetwork.InterfaceInfos{{
		InterfaceName:       "eth0",
		InterfaceType:       corenetwork.EthernetDevice,
		ConfigType:          corenetwork.ConfigDHCP,
		ParentInterfaceName: network.DefaultLXDBridge,
	}})
	_, _, err = s.manager.CreateContainer(
		context.Background(), iCfg, constraints.Value{}, corebase.MakeDefaultBase("ubuntu", "16.04"), netConfig, &container.StorageConfig{}, lxdtesting.NoOpCallback,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *managerSuite) TestCreateContainerCreateFailed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectCreateRemoteOp(ctrl, &lxdapi.Operation{StatusCode: lxdapi.Failure, Err: "create failed"})

	image := lxdapi.Image{Filename: "this-is-our-image"}
	s.expectGetImage(image, nil)

	exp := s.cSvr.EXPECT()
	exp.CreateInstanceFromImage(s.cSvr, image, gomock.Any()).Return(s.createRemoteOp, nil)

	s.makeManager(c)
	_, _, err := s.manager.CreateContainer(
		context.Background(),
		prepInstanceConfig(c),
		constraints.Value{},
		corebase.MakeDefaultBase("ubuntu", "16.04"),
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*create failed")
}

func (s *managerSuite) TestCreateContainerSpecCreationError(c *gc.C) {
	defer s.setup(c).Finish()

	// When the local image acquisition fails, this will cause the remote
	// connection attempt to fail.
	// This is our error condition exit from manager.getContainerSpec.
	lxd.PatchConnectRemote(s, map[string]lxdclient.ImageServer{})

	image := lxdapi.Image{Filename: "this-is-our-image"}
	s.expectGetImage(image, errors.New("not here"))

	s.makeManager(c)
	_, _, err := s.manager.CreateContainer(
		context.Background(),
		prepInstanceConfig(c),
		constraints.Value{},
		corebase.MakeDefaultBase("ubuntu", "16.04"),
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*unrecognized remote server")
}

func (s *managerSuite) TestCreateContainerStartFailed(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.patch()
	s.makeManager(c)

	iCfg := prepInstanceConfig(c)
	hostName, err := s.manager.Namespace().Hostname(iCfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	s.expectUpdateOp(ctrl, "", errors.New("start failed"))
	s.expectDeleteOp(ctrl)
	s.expectCreateContainer(ctrl)

	exp := s.cSvr.EXPECT()
	gomock.InOrder(
		exp.UpdateInstanceState(
			hostName, lxdapi.InstanceStatePut{Action: "start", Timeout: -1}, "").Return(s.updateOp, nil),
		exp.GetInstanceState(hostName).Return(&lxdapi.InstanceState{StatusCode: lxdapi.Stopped}, lxdtesting.ETag, nil),
		exp.DeleteInstance(hostName).Return(s.deleteOp, nil),
	)

	_, _, err = s.manager.CreateContainer(
		context.Background(),
		iCfg,
		constraints.Value{},
		corebase.MakeDefaultBase("ubuntu", "16.04"),
		prepNetworkConfig(),
		&container.StorageConfig{},
		lxdtesting.NoOpCallback,
	)
	c.Assert(err, gc.ErrorMatches, ".*start failed")
}

func (s *managerSuite) TestListContainers(c *gc.C) {
	defer s.setup(c).Finish()
	s.makeManager(c)

	prefix := s.manager.Namespace().Prefix()
	wrongPrefix := prefix[:len(prefix)-1] + "j"

	containers := []lxdapi.Instance{
		{Name: "foobar", Type: "container"},
		{Name: "definitely-not-a-juju-container", Type: "container"},
		{Name: wrongPrefix + "-0", Type: "container"},
		{Name: prefix + "-0", Type: "container"},
		{Name: "please-disperse", Type: "container"},
		{Name: prefix + "-1", Type: "container"},
		{Name: "nothing-to-see-here-please", Type: "container"},
	}

	s.cSvr.EXPECT().GetInstances(lxdapi.InstanceTypeAny).Return(containers, nil)

	result, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 2)
	c.Check(string(result[0].Id()), gc.Equals, prefix+"-0")
	c.Check(string(result[1].Id()), gc.Equals, prefix+"-1")
}

func (s *managerSuite) TestIsInitialized(c *gc.C) {
	mgr, err := lxd.NewContainerManager(getBaseConfig(), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mgr.IsInitialized(), gc.Equals, lxd.SocketPath(lxd.IsUnixSocket) != "")
}

func (s *managerSuite) TestNetworkDevicesFromConfigWithEmptyParentDevice(c *gc.C) {
	defer s.setup(c).Finish()

	interfaces := corenetwork.InterfaceInfos{{
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           9000,
	}}
	s.makeManager(c)
	result, _, err := lxd.NetworkDevicesFromConfig(s.manager, &container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, gc.ErrorMatches, "parent interface name is empty")
	c.Assert(result, gc.IsNil)
}

func (s *managerSuite) TestNetworkDevicesFromConfigWithParentDevice(c *gc.C) {
	defer s.setup(c).Finish()

	interfaces := corenetwork.InterfaceInfos{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		MACAddress:          "aa:bb:cc:dd:ee:f0",
		Addresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("", corenetwork.WithCIDR("10.10.0.0/24")).AsProviderAddress(),
		},
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

	s.makeManager(c)
	result, unknown, err := lxd.NetworkDevicesFromConfig(s.manager, &container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
	c.Check(unknown, gc.HasLen, 0)
}

func (s *managerSuite) TestNetworkDevicesFromConfigUnknownCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	interfaces := corenetwork.InterfaceInfos{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}}

	s.makeManager(c)
	_, unknown, err := lxd.NetworkDevicesFromConfig(s.manager, &container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(unknown, gc.DeepEquals, []string{"br-eth0"})
}

func (s *managerSuite) TestNetworkDevicesFromConfigNoInputGetsProfileNICs(c *gc.C) {
	defer s.setup(c).Finish()
	s.patch()

	s.cSvr.EXPECT().GetProfile("default").Return(defaultLegacyProfileWithNIC(), lxdtesting.ETag, nil)

	s.makeManager(c)
	result, _, err := lxd.NetworkDevicesFromConfig(s.manager, &container.NetworkConfig{})
	c.Assert(err, jc.ErrorIsNil)

	exp := map[string]map[string]string{
		"eth0": {
			"parent":  network.DefaultLXDBridge,
			"type":    "nic",
			"nictype": "bridged",
			"hwaddr":  "00:16:3e:00:00:00",
			// NOTE: the host name will not be set because we get
			// the NICs from the default profile.
		},
	}

	c.Check(result, gc.DeepEquals, exp)
}

func (s *managerSuite) TestGetImageSourcesDefaultConfig(c *gc.C) {
	defer s.setup(c).Finish()

	s.makeManager(c)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote, lxd.CloudImagesLinuxContainersRemote})
}

func (s *managerSuite) TestGetImageSourcesNoDefaults(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataDefaultsDisabledKey] = "true"
	s.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.HasLen, 0)
}

func (s *managerSuite) TestGetImageSourcesNoDefaultsCustomURL(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataDefaultsDisabledKey] = "true"
	cfg[config.ContainerImageMetadataURLKey] = "https://special.container.sauce"
	s.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)
	expectedSources := []lxd.ServerSpec{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (s *managerSuite) TestGetImageSourcesNonStandardStreamDefaultConfig(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "nope"
	s.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote, lxd.CloudImagesLinuxContainersRemote})
}

func (s *managerSuite) TestGetImageSourcesDailyOnly(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "daily"
	s.makeManagerForConfig(c, cfg)
	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxd.ServerSpec{lxd.CloudImagesDailyRemote, lxd.CloudImagesLinuxContainersRemote})
}

func (s *managerSuite) TestGetImageSourcesImageMetadataURLExpectedHTTPSSources(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	s.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.ServerSpec{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
		lxd.CloudImagesRemote,
		lxd.CloudImagesDailyRemote,
		lxd.CloudImagesLinuxContainersRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (s *managerSuite) TestGetImageSourcesImageMetadataURLDailyStream(c *gc.C) {
	defer s.setup(c).Finish()

	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	cfg[config.ContainerImageStreamKey] = "daily"
	s.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(s.manager)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxd.ServerSpec{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxd.SimpleStreamsProtocol,
		},
		lxd.CloudImagesDailyRemote,
		lxd.CloudImagesLinuxContainersRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (s *managerSuite) TestMaybeWriteLXDProfile(c *gc.C) {
	defer s.setup(c).Finish()

	s.makeManager(c)
	proMgr, ok := s.manager.(container.LXDProfileManager)
	c.Assert(ok, jc.IsTrue)

	put := lxdprofile.Profile{
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
	s.cSvr.EXPECT().CreateProfile(post).Return(nil)
	s.cSvr.EXPECT().GetProfileNames().Return([]string{"default", "custom"}, nil)
	expProfile := lxdapi.Profile{
		Name:        post.Name,
		Config:      post.Config,
		Description: post.Description,
		Devices:     post.Devices,
	}
	s.cSvr.EXPECT().GetProfile(post.Name).Return(&expProfile, "etag", nil)

	err := proMgr.MaybeWriteLXDProfile("juju-default-lxd-0", put)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *managerSuite) TestAssignLXDProfiles(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectUpdateOp(ctrl, "Updating container", nil)

	old := "old-profile"
	new := "new-profile"
	newProfiles := []string{"default", "juju-default", new}
	put := lxdprofile.Profile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "test profile",
	}
	s.expectUpdateContainerProfiles(old, new, newProfiles, lxdapi.ProfilePut(put))
	profilePosts := []lxdprofile.ProfilePost{
		{
			Name:    old,
			Profile: nil,
		}, {
			Name:    new,
			Profile: &put,
		},
	}

	s.makeManager(c)
	proMgr, ok := s.manager.(container.LXDProfileManager)
	c.Assert(ok, jc.IsTrue)

	obtained, err := proMgr.AssignLXDProfiles("testme", newProfiles, profilePosts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, newProfiles)
}

func (s *managerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.cSvr = s.NewMockServer(ctrl)
	return ctrl
}

func (s *managerSuite) setupWithExtensions(c *gc.C, extensions ...string) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.cSvr = s.NewMockServerWithExtensions(ctrl, extensions...)
	return ctrl
}

// expectCreateContainer is a convenience function for the expectations
// concerning a successful container creation based on a cached local
// image.
func (s *managerSuite) expectCreateContainer(ctrl *gomock.Controller) {
	s.expectCreateRemoteOp(ctrl, &lxdapi.Operation{StatusCode: lxdapi.Success})

	image := lxdapi.Image{Filename: "this-is-our-image"}
	s.expectGetImage(image, nil)

	exp := s.cSvr.EXPECT()
	exp.CreateInstanceFromImage(s.cSvr, image, gomock.Any()).Return(s.createRemoteOp, nil)
}

// expectCreateRemoteOp is a convenience function for the expectations
// concerning successful remote operations.
func (s *managerSuite) expectCreateRemoteOp(ctrl *gomock.Controller, op *lxdapi.Operation) {
	s.createRemoteOp = lxdtesting.NewMockRemoteOperation(ctrl)
	s.createRemoteOp.EXPECT().Wait().Return(nil).AnyTimes()
	s.createRemoteOp.EXPECT().GetTarget().Return(op, nil)
}

// expectDeleteOp is a convenience function for the expectations
// concerning successful delete operations.
func (s *managerSuite) expectDeleteOp(ctrl *gomock.Controller) {
	s.deleteOp = lxdtesting.NewMockOperation(ctrl)
	s.deleteOp.EXPECT().Wait().Return(nil).AnyTimes()
}

// expectDeleteOp is a convenience function for the expectations
// concerning GetImage operations.
func (s *managerSuite) expectGetImage(image lxdapi.Image, getImageErr error) {
	target := "foo-target"
	alias := &lxdapi.ImageAliasesEntry{Target: target}

	exp := s.cSvr.EXPECT()
	gomock.InOrder(
		exp.GetImageAlias("juju/ubuntu@16.04/"+s.Arch()).Return(alias, lxdtesting.ETag, nil),
		exp.GetImage(target).Return(&image, lxdtesting.ETag, getImageErr),
	)
}

// expectStartOp is a convenience function for the expectations
// concerning a successful start operation.
func (s *managerSuite) expectStartOp(ctrl *gomock.Controller) {
	s.startOp = lxdtesting.NewMockOperation(ctrl)
	s.startOp.EXPECT().Wait().Return(nil)
}

// expectStopOp is a convenience function for the expectations
// concerning successful stop operation.
func (s *managerSuite) expectStopOp(ctrl *gomock.Controller) {
	s.stopOp = lxdtesting.NewMockOperation(ctrl)
	s.stopOp.EXPECT().Wait().Return(nil)
}

// expectStopOp is a convenience function for the expectations
// concerning an update operation.
func (s *managerSuite) expectUpdateOp(ctrl *gomock.Controller, description string, waitErr error) {
	s.updateOp = lxdtesting.NewMockOperation(ctrl)
	s.updateOp.EXPECT().Wait().Return(waitErr)
	if waitErr != nil {
		return
	}
	s.updateOp.EXPECT().Get().Return(lxdapi.Operation{Description: description})
}

func (s *managerSuite) expectUpdateContainerProfiles(old, new string, newProfiles []string, put lxdapi.ProfilePut) {
	instId := "testme"
	oldProfiles := []string{"default", "juju-default", old}
	post := lxdapi.ProfilesPost{
		ProfilePut: put,
		Name:       new,
	}
	expProfile := lxdapi.Profile{
		Name:        post.Name,
		Description: post.Description,
		Config:      post.Config,
		Devices:     post.Devices,
	}
	cExp := s.cSvr.EXPECT()
	gomock.InOrder(
		cExp.GetProfileNames().Return(oldProfiles, nil),
		cExp.CreateProfile(post).Return(nil),
		cExp.GetProfile(post.Name).Return(&expProfile, "etag", nil),
		cExp.GetInstance(instId).Return(
			&lxdapi.Instance{
				Profiles: oldProfiles,
			}, "", nil),
		cExp.UpdateInstance(instId, gomock.Any(), gomock.Any()).Return(s.updateOp, nil),
		cExp.DeleteProfile(old).Return(nil),
		cExp.GetInstance(instId).Return(
			&lxdapi.Instance{
				Profiles: newProfiles,
			}, "", nil),
	)
}
