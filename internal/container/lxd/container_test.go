// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net/http"
	"time"

	"github.com/canonical/lxd/shared/api"
	"github.com/canonical/lxd/shared/osarch"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/container/lxd/mocks"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
	"github.com/juju/juju/internal/network"
)

type containerSuite struct {
	lxdtesting.BaseSuite
}

var _ = tc.Suite(&containerSuite{})

func (s *containerSuite) TestContainerMetadata(c *tc.C) {
	container := lxd.Container{}
	container.Config = map[string]string{"user.juju-controller-uuid": "something"}
	c.Check(container.Metadata(tags.JujuController), tc.Equals, "something")
}

func (s *containerSuite) TestContainerArch(c *tc.C) {
	lxdArch, _ := osarch.ArchitectureName(osarch.ARCH_64BIT_INTEL_X86)
	container := lxd.Container{}
	container.Architecture = lxdArch
	c.Check(container.Arch(), tc.Equals, arch.AMD64)
}

func (s *containerSuite) TestContainerVirtType(c *tc.C) {
	// This test locks in the fact the names of the instance types are the
	// same as the api instance types.
	container := lxd.Container{}
	container.Type = string(instance.DefaultInstanceType)
	c.Check(container.VirtType().String(), tc.Equals, string(api.InstanceTypeContainer))
	container.Type = string(instance.InstanceTypeContainer)
	c.Check(container.VirtType().String(), tc.Equals, string(api.InstanceTypeContainer))
	container.Type = string(instance.InstanceTypeVM)
	c.Check(container.VirtType().String(), tc.Equals, string(api.InstanceTypeVM))
	container.Type = string(instance.AnyInstanceType)
	c.Check(container.VirtType().String(), tc.Equals, string(api.InstanceTypeAny))
}

func (s *containerSuite) TestContainerCPUs(c *tc.C) {
	container := lxd.Container{}
	container.Config = map[string]string{"limits.cpu": "2"}
	c.Check(container.CPUs(), tc.Equals, uint(2))
}

func (s *containerSuite) TestContainerMem(c *tc.C) {
	container := lxd.Container{}

	container.Config = map[string]string{"limits.memory": "1MiB"}
	c.Check(int(container.Mem()), tc.Equals, 1)

	container.Config = map[string]string{"limits.memory": "2GiB"}
	c.Check(int(container.Mem()), tc.Equals, 2048)
}

func (s *containerSuite) TestContainerAddDiskNoDevices(c *tc.C) {
	container := lxd.Container{}
	err := container.AddDisk("root", "/", "source", "default", true)
	c.Assert(err, jc.ErrorIsNil)

	expected := map[string]string{
		"type":     "disk",
		"path":     "/",
		"source":   "source",
		"pool":     "default",
		"readonly": "true",
	}
	c.Check(container.Devices["root"], tc.DeepEquals, expected)
}

func (s *containerSuite) TestContainerAddDiskDevicePresentError(c *tc.C) {
	container := lxd.Container{}
	container.Name = "seeyounexttuesday"
	container.Devices = map[string]map[string]string{"root": {}}

	err := container.AddDisk("root", "/", "source", "default", true)
	c.Check(err, tc.ErrorMatches, `container "seeyounexttuesday" already has a device "root"`)
}

func (s *containerSuite) TestFilterContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	matching := []api.Instance{
		{
			Name:       "prefix-c1",
			StatusCode: api.Starting,
		},
		{
			Name:       "prefix-c2",
			StatusCode: api.Stopped,
		},
	}
	ret := append(matching, []api.Instance{
		{
			Name:       "prefix-c3",
			StatusCode: api.Started,
		},
		{
			Name:       "not-prefix-c4",
			StatusCode: api.Stopped,
		},
	}...)
	cSvr.EXPECT().GetInstances(api.InstanceTypeAny).Return(ret, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	filtered, err := jujuSvr.FilterContainers("prefix", "Starting", "Stopped")
	c.Assert(err, jc.ErrorIsNil)

	expected := make([]lxd.Container, len(matching))
	for i, v := range matching {
		expected[i] = lxd.Container{v}
	}

	c.Check(filtered, tc.DeepEquals, expected)
}

func (s *containerSuite) TestAliveContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	matching := []api.Instance{
		{
			Name:       "c1",
			StatusCode: api.Starting,
		},
		{
			Name:       "c2",
			StatusCode: api.Stopped,
		},
		{
			Name:       "c3",
			StatusCode: api.Running,
		},
	}
	ret := append(matching, api.Instance{
		Name:       "c4",
		StatusCode: api.Frozen,
	})
	cSvr.EXPECT().GetInstances(api.InstanceTypeAny).Return(ret, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	filtered, err := jujuSvr.AliveContainers("")
	c.Assert(err, jc.ErrorIsNil)

	expected := make([]lxd.Container, len(matching))
	for i, v := range matching {
		expected[i] = lxd.Container{v}
	}
	c.Check(filtered, tc.DeepEquals, expected)
}

func (s *containerSuite) TestContainerAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	state := api.InstanceState{
		Network: map[string]api.InstanceStateNetwork{
			"eth0": {
				Addresses: []api.InstanceStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.0.8.173",
						Netmask: "24",
						Scope:   "global",
					},
					{
						Family:  "inet6",
						Address: "fe80::216:3eff:fe3b:e582",
						Netmask: "64",
						Scope:   "link",
					},
				},
			},
			"lo": {
				Addresses: []api.InstanceStateNetworkAddress{
					{
						Family:  "inet",
						Address: "127.0.0.1",
						Netmask: "8",
						Scope:   "local",
					},
					{
						Family:  "inet6",
						Address: "::1",
						Netmask: "128",
						Scope:   "local",
					},
				},
			},
			"lxdbr0": {
				Addresses: []api.InstanceStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.0.6.17",
						Netmask: "24",
						Scope:   "global",
					},
					{
						Family:  "inet6",
						Address: "fe80::5c9b:b2ff:feaf:4cf2",
						Netmask: "64",
						Scope:   "link",
					},
				},
			},
		},
	}
	cSvr.EXPECT().GetInstanceState("c1").Return(&state, lxdtesting.ETag, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := jujuSvr.ContainerAddresses("c1")
	c.Assert(err, jc.ErrorIsNil)

	expected := []corenetwork.ProviderAddress{
		corenetwork.NewMachineAddress("10.0.8.173", corenetwork.WithScope(corenetwork.ScopeCloudLocal)).AsProviderAddress(),
	}
	c.Check(addrs, tc.DeepEquals, expected)
}

func (s *containerSuite) TestCreateContainerFromSpecSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	// Operation arrangements.
	createOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createOp.EXPECT().Wait().Return(nil)
	createOp.EXPECT().GetTarget().Return(&api.Operation{StatusCode: api.Success}, nil)

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	// Request data.
	image := api.Image{Filename: "container-image"}
	spec := lxd.ContainerSpec{
		Name: "c1",
		Image: lxd.SourcedImage{
			Image:     &image,
			LXDServer: cSvr,
		},
		Profiles: []string{"default"},
		Devices: map[string]map[string]string{
			"eth0": {
				"parent":  network.DefaultLXDBridge,
				"type":    "nic",
				"nictype": "bridged",
			},
		},
		Config: map[string]string{
			"limits.cpu": "2",
		},
	}

	createReq := api.InstancesPost{
		Name: spec.Name,
		Type: api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	startReq := api.InstanceStatePut{
		Action:   "start",
		Timeout:  -1,
		Force:    false,
		Stateful: false,
	}

	// Container created, started and returned.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateInstanceFromImage(cSvr, image, createReq).Return(createOp, nil),
		exp.UpdateInstanceState(spec.Name, startReq, "").Return(startOp, nil),
		exp.GetInstance(spec.Name).Return(&api.Instance{}, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	container, err := jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(container, tc.NotNil)
}

func (s *containerSuite) TestCreateContainerFromSpecAlreadyExists(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	clock := mocks.NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now())

	// Operation arrangements.
	createOp := lxdtesting.NewMockRemoteOperation(ctrl)

	// Request data.
	image := api.Image{Filename: "container-image"}
	spec := lxd.ContainerSpec{
		Name: "c1",
		Image: lxd.SourcedImage{
			Image:     &image,
			LXDServer: cSvr,
		},
		Profiles: []string{"default"},
		Devices: map[string]map[string]string{
			"eth0": {
				"parent":  network.DefaultLXDBridge,
				"type":    "nic",
				"nictype": "bridged",
			},
		},
		Config: map[string]string{
			"limits.cpu": "2",
		},
	}

	createReq := api.InstancesPost{
		Name: spec.Name,
		Type: api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	// Container created, started and returned.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateInstanceFromImage(cSvr, image, createReq).Return(createOp, errors.Errorf("Container 'juju-5bcbde-5-lxd-6' already exists")),
		exp.GetInstance(spec.Name).Return(&api.Instance{
			Profiles:   spec.Profiles,
			Devices:    spec.Devices,
			Config:     spec.Config,
			StatusCode: api.Running,
		}, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewTestingServer(cSvr, clock)
	c.Assert(err, jc.ErrorIsNil)

	container, err := jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(container, tc.NotNil)
}

func (s *containerSuite) TestCreateContainerFromSpecAlreadyExistsNotCorrectSpec(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)
	clock := mocks.NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now())

	// Operation arrangements.
	createOp := lxdtesting.NewMockRemoteOperation(ctrl)

	// Request data.
	image := api.Image{Filename: "container-image"}
	spec := lxd.ContainerSpec{
		Name: "c1",
		Image: lxd.SourcedImage{
			Image:     &image,
			LXDServer: cSvr,
		},
		Profiles: []string{"default"},
		Devices: map[string]map[string]string{
			"eth0": {
				"parent":  network.DefaultLXDBridge,
				"type":    "nic",
				"nictype": "bridged",
			},
		},
		Config: map[string]string{
			"limits.cpu": "2",
		},
	}

	createReq := api.InstancesPost{
		Name: spec.Name,
		Type: api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	// Container created, started and returned.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateInstanceFromImage(cSvr, image, createReq).Return(createOp, errors.Errorf("Container 'juju-5bcbde-5-lxd-6' already exists")),
		exp.GetInstance(spec.Name).Return(&api.Instance{
			StatusCode: api.Running,
		}, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewTestingServer(cSvr, clock)
	c.Assert(err, jc.ErrorIsNil)

	_, err = jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, tc.ErrorMatches, `Container 'juju-5bcbde-5-lxd-6' already exists`)
}

func (s *containerSuite) TestCreateContainerFromSpecStartFailed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	// Operation arrangements.
	createOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createOp.EXPECT().Wait().Return(nil)
	createOp.EXPECT().GetTarget().Return(&api.Operation{StatusCode: api.Success}, nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	// Request data.
	image := api.Image{Filename: "container-image"}
	spec := lxd.ContainerSpec{
		Name: "c1",
		Image: lxd.SourcedImage{
			Image:     &image,
			LXDServer: cSvr,
		},
		Profiles: []string{"default"},
		Devices: map[string]map[string]string{
			"eth0": {
				"parent":  network.DefaultLXDBridge,
				"type":    "nic",
				"nictype": "bridged",
			},
		},
		Config: map[string]string{
			"limits.cpu": "2",
		},
	}

	createReq := api.InstancesPost{
		Name: spec.Name,
		Type: api.InstanceTypeContainer,
		InstancePut: api.InstancePut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	startReq := api.InstanceStatePut{
		Action:   "start",
		Timeout:  -1,
		Force:    false,
		Stateful: false,
	}

	// Container created, starting fails, container state checked, container deleted.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateInstanceFromImage(cSvr, image, createReq).Return(createOp, nil),
		exp.UpdateInstanceState(spec.Name, startReq, "").Return(nil, errors.New("start failed")),
		exp.GetInstanceState(spec.Name).Return(
			&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil),
		exp.DeleteInstance(spec.Name).Return(deleteOp, nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	container, err := jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, tc.ErrorMatches, "start failed")
	c.Check(container, tc.IsNil)
}

func (s *containerSuite) TestRemoveContainersSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil).Times(2)

	stopReq := api.InstanceStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1 is already stopped. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetInstanceState("c1").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c1").Return(deleteOp, nil)
	exp.GetInstanceState("c2").Return(&api.InstanceState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateInstanceState("c2", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteInstance("c2").Return(deleteOp, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerSuite) TestRemoveContainersSuccessWithNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	stopReq := api.InstanceStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1 is already stopped. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetInstanceState("c1").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c1").Return(deleteOp, nil)
	exp.GetInstanceState("c2").Return(&api.InstanceState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateInstanceState("c2", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteInstance("c2").Return(deleteOp, api.StatusErrorf(http.StatusNotFound, ""))

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerSuite) TestRemoveContainersPartialFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	stopReq := api.InstanceStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1, c2 already stopped, but delete fails. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetInstanceState("c1").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c1").Return(nil, errors.New("deletion failed"))
	exp.GetInstanceState("c2").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c2").Return(nil, errors.New("deletion failed"))
	exp.GetInstanceState("c3").Return(&api.InstanceState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateInstanceState("c3", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteInstance("c3").Return(deleteOp, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2", "c3"})
	c.Assert(err, tc.ErrorMatches, "failed to remove containers: c1, c2")
}

func (s *containerSuite) TestDeleteInstancesPartialFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	deleteOpFail := lxdtesting.NewMockOperation(ctrl)
	deleteOpFail.EXPECT().Wait().Return(errors.New("failure")).AnyTimes()

	deleteOpSuccess := lxdtesting.NewMockOperation(ctrl)
	deleteOpSuccess.EXPECT().Wait().Return(nil)

	retries := 3

	// Container c1, c2 already stopped, but delete fails. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetInstanceState("c1").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c1").Return(deleteOpFail, nil)
	exp.DeleteInstance("c1").Return(deleteOpSuccess, nil)

	exp.GetInstanceState("c2").Return(&api.InstanceState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteInstance("c2").Return(deleteOpFail, nil).Times(retries)

	clock := mocks.NewMockClock(ctrl)
	ch := make(chan time.Time)

	cExp := clock.EXPECT()
	cExp.Now().Return(time.Now()).AnyTimes()
	cExp.After(2 * time.Second).Return(ch).AnyTimes()

	go func() {
		for i := 0; i < retries; i++ {
			ch <- time.Now()
		}
	}()

	jujuSvr, err := lxd.NewTestingServer(cSvr, clock)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2"})
	c.Assert(err, tc.ErrorMatches, "failed to remove containers: c2")
}

func (s *managerSuite) TestSpecApplyConstraints(c *tc.C) {
	mem := uint64(2046)
	cores := uint64(4)
	instType := "t2.micro"

	cons := constraints.Value{
		Mem:          &mem,
		CpuCores:     &cores,
		InstanceType: &instType,
	}

	spec := lxd.ContainerSpec{
		Config: map[string]string{lxd.AutoStartKey: "true"},
	}

	// Uses the "MiB" suffix.
	exp := map[string]string{
		lxd.AutoStartKey: "true",
		"limits.memory":  "2046MiB",
		"limits.cpu":     "4",
	}
	spec.ApplyConstraints("3.10.0", cons)
	c.Check(spec.Config, tc.DeepEquals, exp)
	c.Check(spec.InstanceType, tc.Equals, instType)
}
