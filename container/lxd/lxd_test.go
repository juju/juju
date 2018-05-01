// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"
	"sync"
	stdtesting "testing"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/tools/lxdtools"
	"github.com/juju/juju/tools/lxdtools/testmock"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type LxdSuite struct{}

var _ = gc.Suite(&LxdSuite{})

func (t *LxdSuite) makeManager(c *gc.C) container.Manager {
	return t.makeManagerForConfig(c, getBaseConfig())
}

func (t *LxdSuite) makeManagerForConfig(c *gc.C, cfg container.ManagerConfig) container.Manager {
	manager, err := lxd.NewContainerManager(cfg)
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

func (t *LxdSuite) TestContainerCreateDestroy(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	containerServer := testmock.NewMockContainerServer(mockCtrl)
	mockedImage := lxdapi.Image{Filename: "this-is-our-image"}

	lxd.ConnectLocal = func() (lxdclient.ContainerServer, error) {
		return containerServer, nil
	}
	manager := t.makeManager(c)
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error {
		return nil
	}
	icfg := prepInstanceConfig(c)
	ncfg := prepNetworkConfig()
	hostname, err := manager.Namespace().Hostname(icfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	// launch instance
	chDone := make(chan bool, 1)
	chDone <- true
	createOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Success}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)
	remoteCreateOperation := lxdclient.NewRemoteOperation(&createOperation, []func(lxdapi.Operation){}, chDone, nil, nil)
	updateOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Success}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)

	gomock.InOrder(
		containerServer.EXPECT().GetImageAlias("juju/xenial/amd64").Return(&lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}, "ETAG", nil),
		containerServer.EXPECT().GetImage("foo-target").Return(&mockedImage, "ETAG", nil),
		containerServer.EXPECT().CreateContainerFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(&remoteCreateOperation, nil),
		containerServer.EXPECT().UpdateContainerState(hostname, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(&updateOperation, nil),
	)
	instance, hc, err := manager.CreateContainer(
		icfg,
		constraints.Value{},
		"xenial",
		ncfg,
		&container.StorageConfig{},
		callback,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check status
	instanceId := instance.Id()
	c.Check(string(instanceId), gc.Equals, hostname)
	gomock.InOrder(
		containerServer.EXPECT().GetContainerState(hostname).Return(&lxdapi.ContainerState{StatusCode: lxdapi.Running}, "ETAG", nil),
	)
	instanceStatus := instance.Status(context.NewCloudCallContext())
	c.Check(instanceStatus.Status, gc.Equals, status.Running)
	c.Check(*hc.AvailabilityZone, gc.Equals, "test-availability-zone")

	// Remove instance
	deleteOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Success}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)
	gomock.InOrder(
		containerServer.EXPECT().GetContainerState(hostname).Return(&lxdapi.ContainerState{StatusCode: lxdapi.Running}, "ETAG", nil),
		containerServer.EXPECT().UpdateContainerState(hostname, lxdapi.ContainerStatePut{Action: "stop", Timeout: -1}, "ETAG").Return(&updateOperation, nil),
		containerServer.EXPECT().DeleteContainer(hostname).Return(&deleteOperation, nil),
	)
	err = manager.DestroyContainer(instanceId)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *LxdSuite) TestCreateContainerCreateFailed(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	containerServer := testmock.NewMockContainerServer(mockCtrl)
	mockedImage := lxdapi.Image{Filename: "this-is-our-image"}

	lxd.ConnectLocal = func() (lxdclient.ContainerServer, error) {
		return containerServer, nil
	}
	manager := t.makeManager(c)
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error {
		return nil
	}
	icfg := prepInstanceConfig(c)
	ncfg := prepNetworkConfig()

	chDone := make(chan bool, 1)
	chDone <- true
	createOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Failure, Err: "create failed"}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)
	remoteCreateOperation := lxdclient.NewRemoteOperation(&createOperation, []func(lxdapi.Operation){}, chDone, nil, nil)

	gomock.InOrder(
		containerServer.EXPECT().GetImageAlias("juju/xenial/amd64").Return(&lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}, "ETAG", nil),
		containerServer.EXPECT().GetImage("foo-target").Return(&mockedImage, "ETAG", nil),
		containerServer.EXPECT().CreateContainerFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(&remoteCreateOperation, nil),
	)

	_, _, err := manager.CreateContainer(
		icfg,
		constraints.Value{},
		"xenial",
		ncfg,
		&container.StorageConfig{},
		callback,
	)
	c.Assert(err, gc.ErrorMatches, ".*create failed")
}

func (t *LxdSuite) TestCreateContainerStartFailed(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	containerServer := testmock.NewMockContainerServer(mockCtrl)
	mockedImage := lxdapi.Image{Filename: "this-is-our-image"}

	lxd.ConnectLocal = func() (lxdclient.ContainerServer, error) {
		return containerServer, nil
	}

	manager := t.makeManager(c)
	callback := func(settableStatus status.Status, info string, data map[string]interface{}) error {
		return nil
	}

	icfg := prepInstanceConfig(c)
	ncfg := prepNetworkConfig()
	hostname, err := manager.Namespace().Hostname(icfg.MachineId)
	c.Assert(err, jc.ErrorIsNil)

	chDone := make(chan bool, 1)
	chDone <- true
	createOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Success}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)
	remoteCreateOperation := lxdclient.NewRemoteOperation(&createOperation, []func(lxdapi.Operation){}, chDone, nil, nil)
	updateOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Failure, Err: "start failed"}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)
	deleteOperation := lxdclient.NewOperation(lxdapi.Operation{StatusCode: lxdapi.Success}, nil, &lxdclient.EventListener{}, true, sync.Mutex{}, nil)

	gomock.InOrder(
		containerServer.EXPECT().GetImageAlias("juju/xenial/amd64").Return(&lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}, "ETAG", nil),
		containerServer.EXPECT().GetImage("foo-target").Return(&mockedImage, "ETAG", nil),
		containerServer.EXPECT().CreateContainerFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(&remoteCreateOperation, nil),
		containerServer.EXPECT().UpdateContainerState(hostname, lxdapi.ContainerStatePut{Action: "start", Timeout: -1}, "").Return(&updateOperation, nil),
		containerServer.EXPECT().DeleteContainer(hostname).Return(&deleteOperation, nil),
	)

	_, _, err = manager.CreateContainer(
		icfg,
		constraints.Value{},
		"xenial",
		ncfg,
		&container.StorageConfig{},
		callback,
	)
	c.Assert(err, gc.ErrorMatches, ".*start failed")
}

func (t *LxdSuite) TestListContainers(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	containerServer := testmock.NewMockContainerServer(mockCtrl)
	lxd.ConnectLocal = func() (lxdclient.ContainerServer, error) {
		return containerServer, nil
	}
	manager := t.makeManager(c)

	prefix := manager.Namespace().Prefix()
	wrongPrefix := prefix[:len(prefix)-1] + "j"

	containers := []lxdapi.Container{
		lxdapi.Container{Name: "foobar"},
		lxdapi.Container{Name: "definitely-not-a-juju-container"},
		lxdapi.Container{Name: wrongPrefix + "-0"},
		lxdapi.Container{Name: prefix + "-0"},
		lxdapi.Container{Name: "please-disperse"},
		lxdapi.Container{Name: prefix + "-1"},
		lxdapi.Container{Name: "nothing-to-see-here-please"},
	}
	gomock.InOrder(
		containerServer.EXPECT().GetContainers().Return(containers, nil),
	)
	result, err := manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 2)
	c.Check(string(result[0].Id()), gc.Equals, prefix+"-0")
	c.Check(string(result[1].Id()), gc.Equals, prefix+"-1")
}

func (t *LxdSuite) TestIsInitialized(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	containerServer := testmock.NewMockContainerServer(mockCtrl)
	attempt := 0
	lxd.ConnectLocal = func() (lxdclient.ContainerServer, error) {
		defer func() { attempt = attempt + 1 }()
		if attempt == 0 {
			return nil, errors.New("failed to connect")
		} else {
			return containerServer, nil
		}
	}
	manager := t.makeManager(c)
	c.Check(manager.IsInitialized(), gc.Equals, false)
	c.Check(manager.IsInitialized(), gc.Equals, true)
	c.Check(manager.IsInitialized(), gc.Equals, true)
	// We check that we connected only once
	c.Check(attempt, gc.Equals, 2)
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
	expected := map[string]string{
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
	expected := map[string]string{
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
	expected := map[string]string{
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

func (t *LxdSuite) TestNetworkDevicesWithEmptyParentDevice(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		InterfaceName: "eth1",
		InterfaceType: "ethernet",
		Address:       network.NewAddress("0.10.0.21"),
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		MTU:           9000,
	}}

	result, err := lxd.NetworkDevices(&container.NetworkConfig{
		Interfaces: interfaces,
	})

	c.Assert(err, gc.ErrorMatches, "invalid parent device name")
	c.Assert(result, gc.IsNil)
}

func (t *LxdSuite) TestNetworkDevicesWithParentDevice(c *gc.C) {
	interfaces := []network.InterfaceInfo{{
		ParentInterfaceName: "br-eth0",
		InterfaceName:       "eth0",
		InterfaceType:       "ethernet",
		Address:             network.NewAddress("0.10.0.20"),
		MACAddress:          "aa:bb:cc:dd:ee:f0",
	}}

	expected := map[string]map[string]string{
		"eth0": map[string]string{
			"hwaddr":  "aa:bb:cc:dd:ee:f0",
			"name":    "eth0",
			"nictype": "bridged",
			"parent":  "br-eth0",
			"type":    "nic",
		},
	}

	result, err := lxd.NetworkDevices(&container.NetworkConfig{
		Device:     "lxdbr0",
		Interfaces: interfaces,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}

func (t *LxdSuite) TestGetImageSourcesDefaultConfig(c *gc.C) {
	mgr := t.makeManager(c)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdtools.RemoteServer{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesNonStandardStreamDefaultConfig(c *gc.C) {
	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "nope"
	mgr := t.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdtools.RemoteServer{lxd.CloudImagesRemote, lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesDailyOnly(c *gc.C) {
	cfg := getBaseConfig()
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sources, gc.DeepEquals, []lxdtools.RemoteServer{lxd.CloudImagesDailyRemote})
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLExpectedHTTPSSources(c *gc.C) {
	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	mgr := t.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxdtools.RemoteServer{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxdtools.SimplestreamsProtocol,
		},
		lxd.CloudImagesRemote,
		lxd.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}

func (t *LxdSuite) TestGetImageSourcesImageMetadataURLDailyStream(c *gc.C) {
	cfg := getBaseConfig()
	cfg[config.ContainerImageMetadataURLKey] = "http://special.container.sauce"
	cfg[config.ContainerImageStreamKey] = "daily"
	mgr := t.makeManagerForConfig(c, cfg)

	sources, err := lxd.GetImageSources(mgr)
	c.Assert(err, jc.ErrorIsNil)

	expectedSources := []lxdtools.RemoteServer{
		{
			Name:     "special.container.sauce",
			Host:     "https://special.container.sauce",
			Protocol: lxdtools.SimplestreamsProtocol,
		},
		lxd.CloudImagesDailyRemote,
	}
	c.Check(sources, gc.DeepEquals, expectedSources)
}
