// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/lxc/lxd/shared/api"
	"github.com/lxc/lxd/shared/osarch"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/container/lxd/mocks"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	"github.com/juju/juju/core/constraints"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/network"
)

type containerSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&containerSuite{})

func (s *containerSuite) TestContainerMetadata(c *gc.C) {
	container := lxd.Container{}
	container.Config = map[string]string{"user.juju-controller-uuid": "something"}
	c.Check(container.Metadata(tags.JujuController), gc.Equals, "something")
}

func (s *containerSuite) TestContainerArch(c *gc.C) {
	lxdArch, _ := osarch.ArchitectureName(osarch.ARCH_64BIT_INTEL_X86)
	container := lxd.Container{}
	container.Architecture = lxdArch
	c.Check(container.Arch(), gc.Equals, arch.AMD64)
}

func (s *containerSuite) TestContainerCPUs(c *gc.C) {
	container := lxd.Container{}
	container.Config = map[string]string{"limits.cpu": "2"}
	c.Check(container.CPUs(), gc.Equals, uint(2))
}

func (s *containerSuite) TestContainerMem(c *gc.C) {
	container := lxd.Container{}

	container.Config = map[string]string{"limits.memory": "1MiB"}
	c.Check(int(container.Mem()), gc.Equals, 1)

	container.Config = map[string]string{"limits.memory": "2GiB"}
	c.Check(int(container.Mem()), gc.Equals, 2048)
}

func (s *containerSuite) TestContainerAddDiskNoDevices(c *gc.C) {
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
	c.Check(container.Devices["root"], gc.DeepEquals, expected)
}

func (s *containerSuite) TestContainerAddDiskDevicePresentError(c *gc.C) {
	container := lxd.Container{}
	container.Name = "seeyounexttuesday"
	container.Devices = map[string]map[string]string{"root": {}}

	err := container.AddDisk("root", "/", "source", "default", true)
	c.Check(err, gc.ErrorMatches, `container "seeyounexttuesday" already has a device "root"`)
}

func (s *containerSuite) TestFilterContainers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	matching := []api.Container{
		{
			Name:       "prefix-c1",
			StatusCode: api.Starting,
		},
		{
			Name:       "prefix-c2",
			StatusCode: api.Stopped,
		},
	}
	ret := append(matching, []api.Container{
		{
			Name:       "prefix-c3",
			StatusCode: api.Started,
		},
		{
			Name:       "not-prefix-c4",
			StatusCode: api.Stopped,
		},
	}...)
	cSvr.EXPECT().GetContainers().Return(ret, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	filtered, err := jujuSvr.FilterContainers("prefix", "Starting", "Stopped")
	c.Assert(err, jc.ErrorIsNil)

	expected := make([]lxd.Container, len(matching))
	for i, v := range matching {
		expected[i] = lxd.Container{v}
	}

	c.Check(filtered, gc.DeepEquals, expected)
}

func (s *containerSuite) TestAliveContainers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	matching := []api.Container{
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
	ret := append(matching, api.Container{
		Name:       "c4",
		StatusCode: api.Frozen,
	})
	cSvr.EXPECT().GetContainers().Return(ret, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	filtered, err := jujuSvr.AliveContainers("")
	c.Assert(err, jc.ErrorIsNil)

	expected := make([]lxd.Container, len(matching))
	for i, v := range matching {
		expected[i] = lxd.Container{v}
	}
	c.Check(filtered, gc.DeepEquals, expected)
}

func (s *containerSuite) TestContainerAddresses(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	state := api.ContainerState{
		Network: map[string]api.ContainerStateNetwork{
			"eth0": {
				Addresses: []api.ContainerStateNetworkAddress{
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
				Addresses: []api.ContainerStateNetworkAddress{
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
			"lxcbr0": {
				Addresses: []api.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.0.5.12",
						Netmask: "24",
						Scope:   "global",
					},
					{
						Family:  "inet6",
						Address: "fe80::216:3eff:fe3b:e432",
						Netmask: "64",
						Scope:   "link",
					},
				},
			},
			"lxdbr0": {
				Addresses: []api.ContainerStateNetworkAddress{
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
	cSvr.EXPECT().GetContainerState("c1").Return(&state, lxdtesting.ETag, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := jujuSvr.ContainerAddresses("c1")
	c.Assert(err, jc.ErrorIsNil)

	expected := []corenetwork.ProviderAddress{
		corenetwork.NewScopedProviderAddress("10.0.8.173", corenetwork.ScopeCloudLocal),
	}
	c.Check(addrs, gc.DeepEquals, expected)
}

func (s *containerSuite) TestCreateContainerFromSpecSuccess(c *gc.C) {
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

	createReq := api.ContainersPost{
		Name: spec.Name,
		ContainerPut: api.ContainerPut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	startReq := api.ContainerStatePut{
		Action:   "start",
		Timeout:  -1,
		Force:    false,
		Stateful: false,
	}

	// Container created, started and returned.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateContainerFromImage(cSvr, image, createReq).Return(createOp, nil),
		exp.UpdateContainerState(spec.Name, startReq, "").Return(startOp, nil),
		exp.GetContainer(spec.Name).Return(&api.Container{}, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	container, err := jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(container, gc.NotNil)
}

func (s *containerSuite) TestCreateContainerFromSpecStartFailed(c *gc.C) {
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

	createReq := api.ContainersPost{
		Name: spec.Name,
		ContainerPut: api.ContainerPut{
			Profiles:  spec.Profiles,
			Devices:   spec.Devices,
			Config:    spec.Config,
			Ephemeral: false,
		},
	}

	startReq := api.ContainerStatePut{
		Action:   "start",
		Timeout:  -1,
		Force:    false,
		Stateful: false,
	}

	// Container created, starting fails, container state checked, container deleted.
	exp := cSvr.EXPECT()
	gomock.InOrder(
		exp.CreateContainerFromImage(cSvr, image, createReq).Return(createOp, nil),
		exp.UpdateContainerState(spec.Name, startReq, "").Return(nil, errors.New("start failed")),
		exp.GetContainerState(spec.Name).Return(
			&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil),
		exp.DeleteContainer(spec.Name).Return(deleteOp, nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	container, err := jujuSvr.CreateContainerFromSpec(spec)
	c.Assert(err, gc.ErrorMatches, "start failed")
	c.Check(container, gc.IsNil)
}

func (s *containerSuite) TestRemoveContainersSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil).Times(2)

	stopReq := api.ContainerStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1 is already stopped. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetContainerState("c1").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c1").Return(deleteOp, nil)
	exp.GetContainerState("c2").Return(&api.ContainerState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateContainerState("c2", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteContainer("c2").Return(deleteOp, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerSuite) TestRemoveContainersSuccessWithNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	stopReq := api.ContainerStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1 is already stopped. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetContainerState("c1").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c1").Return(deleteOp, nil)
	exp.GetContainerState("c2").Return(&api.ContainerState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateContainerState("c2", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteContainer("c2").Return(deleteOp, errors.New("not found"))

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerSuite) TestRemoveContainersPartialFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	stopOp := lxdtesting.NewMockOperation(ctrl)
	stopOp.EXPECT().Wait().Return(nil)

	deleteOp := lxdtesting.NewMockOperation(ctrl)
	deleteOp.EXPECT().Wait().Return(nil)

	stopReq := api.ContainerStatePut{
		Action:   "stop",
		Timeout:  -1,
		Force:    true,
		Stateful: false,
	}

	// Container c1, c2 already stopped, but delete fails. Container c2 is started and stopped before deletion.
	exp := cSvr.EXPECT()
	exp.GetContainerState("c1").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c1").Return(nil, errors.New("deletion failed"))
	exp.GetContainerState("c2").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c2").Return(nil, errors.New("deletion failed"))
	exp.GetContainerState("c3").Return(&api.ContainerState{StatusCode: api.Started}, lxdtesting.ETag, nil)
	exp.UpdateContainerState("c3", stopReq, lxdtesting.ETag).Return(stopOp, nil)
	exp.DeleteContainer("c3").Return(deleteOp, nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.RemoveContainers([]string{"c1", "c2", "c3"})
	c.Assert(err, gc.ErrorMatches, "failed to remove containers: c1, c2")
}

func (s *containerSuite) TestDeleteContainersPartialFailure(c *gc.C) {
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
	exp.GetContainerState("c1").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c1").Return(deleteOpFail, nil)
	exp.DeleteContainer("c1").Return(deleteOpSuccess, nil)

	exp.GetContainerState("c2").Return(&api.ContainerState{StatusCode: api.Stopped}, lxdtesting.ETag, nil)
	exp.DeleteContainer("c2").Return(deleteOpFail, nil).Times(retries)

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
	c.Assert(err, gc.ErrorMatches, "failed to remove containers: c2")
}

func (s *managerSuite) TestSpecApplyConstraints(c *gc.C) {
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
	c.Check(spec.Config, gc.DeepEquals, exp)
	c.Check(spec.InstanceType, gc.Equals, instType)

	// Uses the "MB" suffix.
	exp["limits.memory"] = "2046MB"
	spec.ApplyConstraints("2.0.11", cons)
	c.Check(spec.Config, gc.DeepEquals, exp)
	c.Check(spec.InstanceType, gc.Equals, instType)
}
