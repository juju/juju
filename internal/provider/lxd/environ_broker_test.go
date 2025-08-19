// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	containerlxd "github.com/juju/juju/internal/container/lxd"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
	"github.com/juju/juju/internal/provider/lxd"
)

type environBrokerSuite struct {
	lxd.EnvironSuite

	defaultProfile *api.Profile
}

func TestEnvironBrokerSuite(t *testing.T) {
	tc.Run(t, &environBrokerSuite{})
}

func (s *environBrokerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.defaultProfile = &api.Profile{
		Devices: map[string]map[string]string{
			"eth0": {},
		},
	}
}

// containerSpecMatcher is a gomock matcher for testing a container spec
// with a supplied validation func.
type containerSpecMatcher struct {
	check func(spec containerlxd.ContainerSpec) bool
}

func (m containerSpecMatcher) Matches(arg interface{}) bool {
	if spec, ok := arg.(containerlxd.ContainerSpec); ok {
		return m.check(spec)
	}
	return false
}

func (m containerSpecMatcher) String() string {
	return fmt.Sprintf("%T", m.check)
}

func matchesContainerSpec(check func(spec containerlxd.ContainerSpec) bool) gomock.Matcher {
	return containerSpecMatcher{check: check}
}

func (s *environBrokerSuite) TestStartInstanceDefaultNIC(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// Check that no custom devices were passed - vanilla cloud-init.
	check := func(spec containerlxd.ContainerSpec) bool {
		if spec.Config[containerlxd.NetworkConfigKey] != "" {
			return false
		}
		return len(spec.Devices) == 0
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "node01"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	res, err := env.StartInstance(c.Context(), s.GetStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceUseZoneFromServerNameWhenContainerLocationIsNone(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// Check that no custom devices were passed - vanilla cloud-init.
	check := func(spec containerlxd.ContainerSpec) bool {
		if spec.Config[containerlxd.NetworkConfigKey] != "" {
			return false
		}
		return len(spec.Devices) <= 0
	}

	exp := svr.EXPECT()
	exp.Name().Return("node01")
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "none"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	args := s.GetStartInstanceArgs(c)
	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceNonDefaultNIC(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	nics := map[string]map[string]string{
		"eno9": {
			"name":    "eno9",
			"mtu":     "9000",
			"nictype": "bridged",
			"parent":  "lxdbr0",
			"hwaddr":  "00:00:00:00:00",
		},
	}

	// Check that the non-standard devices were passed explicitly,
	// And that we have disabled the standard network config.
	check := func(spec containerlxd.ContainerSpec) bool {
		if !reflect.DeepEqual(spec.Devices, nics) {
			return false
		}
		return spec.Config[containerlxd.NetworkConfigKey] == cloudinit.CloudInitNetworkConfigDisabled
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(nics, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "node01"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	res, err := env.StartInstance(c.Context(), s.GetStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithSubnetsInSpace(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	profileNICs := map[string]map[string]string{
		"eno9": {
			"name":    "eno9",
			"mtu":     "9000",
			"nictype": "bridged",
			"parent":  "lxdbr0",
			"hwaddr":  "00:00:00:00:00",
		},
	}

	// Check that the non-standard devices were passed explicitly,
	// And that we have disabled the standard network config.
	check := func(spec containerlxd.ContainerSpec) bool {
		c.Assert(spec.Devices["eno9"], tc.DeepEquals, profileNICs["eno9"], tc.Commentf("expected NIC from profile to be included"))

		// As the subnet IDs are map keys, the additional generated NIC
		// indices depend on the key iteration order so we need to test
		// both possible variants here.
		matchedNICs := reflect.DeepEqual(spec.Devices, map[string]map[string]string{
			"eno9": profileNICs["eno9"],
			"eth0": {
				"name":    "eth0",
				"type":    "nic",
				"nictype": "bridged",
				"parent":  "ovs-br0",
			},
			"eth1": {
				"name":    "eth1",
				"type":    "nic",
				"nictype": "bridged",
				"parent":  "virbr0",
			},
		}) || reflect.DeepEqual(spec.Devices, map[string]map[string]string{
			"eno9": profileNICs["eno9"],
			"eth0": {
				"name":    "eth0",
				"type":    "nic",
				"nictype": "bridged",
				"parent":  "virbr0",
			},
			"eth1": {
				"name":    "eth1",
				"type":    "nic",
				"nictype": "bridged",
				"parent":  "ovs-br0",
			},
		})
		c.Assert(matchedNICs, tc.IsTrue, tc.Commentf("the expected NICs for space-related subnets were not injected; got %v", spec.Devices))

		return spec.Config[containerlxd.NetworkConfigKey] == cloudinit.CloudInitNetworkConfigDisabled
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(profileNICs, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "node01"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	startArgs := s.GetStartInstanceArgs(c)
	startArgs.SubnetsToZones = []map[network.Id][]string{
		// The following are bogus subnet names that shouldn't
		// normally be reported by Subnets(). They are only
		// here to ensure that assignContainerNICs does not
		// explode if garbage gets passed in.
		{
			"bogus-bridge-10.0.0.0/24": {"locutus"},
			"subnet-bridge":            {"locutus"},
		},
		{
			"subnet-virbr0-10.42.0.0/24": {"locutus"},
			// Bridge name with dashes
			"subnet-ovs-br0-10.0.0.0/24": {"locutus"},
			// Should be ignored as the default profile already
			// specifies a device bridged to lxdbr0
			"subnet-lxdbr0-10.99.0.0/24": {"locutus"},
		},
	}
	res, err := env.StartInstance(c.Context(), startArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementAvailable(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	target := lxdtesting.NewMockInstanceServer(ctrl)
	tExp := target.EXPECT()
	serverRet := &api.Server{}
	image := &api.Image{Filename: "container-image"}

	tExp.GetServer().Return(serverRet, lxdtesting.ETag, nil)
	tExp.GetImageAlias("juju/ubuntu@24.04/amd64").Return(&api.ImageAliasesEntry{}, lxdtesting.ETag, nil)
	tExp.GetImage("").Return(image, lxdtesting.ETag, nil)

	jujuTarget, err := containerlxd.NewServer(target)
	c.Assert(err, tc.ErrorIsNil)

	members := []api.ClusterMember{
		{
			ServerName: "node01",
			Status:     "ONLINE",
		},
		{
			ServerName: "node02",
			Status:     "ONLINE",
		},
	}

	createOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createOp.EXPECT().Wait().Return(nil)
	createOp.EXPECT().GetTarget().Return(&api.Operation{StatusCode: api.Success}, nil)

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
		sExp.IsClustered().Return(true),
		sExp.UseTargetServer(gomock.Any(), "node01").Return(jujuTarget, nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		sExp.HostArch().Return(arch.AMD64),
	)

	// CreateContainerFromSpec is tested in container/lxd.
	// we don't bother with detailed parameter assertions here.
	tExp.CreateInstanceFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(createOp, nil)
	tExp.UpdateInstanceState(gomock.Any(), gomock.Any(), "").Return(startOp, nil)
	tExp.GetInstance(gomock.Any()).Return(&api.Instance{Type: "container", Location: "node01"}, lxdtesting.ETag, nil)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.Placement = "zone=node01"

	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementNotPresent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "ONLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.Placement = "zone=node03"

	_, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `availability zone "node03" not valid`)
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementNotAvailable(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "OFFLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.Placement = "zone=node01"

	_, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "zone \"node01\" is unavailable")
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementBadArgument(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
	)
	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.Placement = "breakfast=eggs"

	_, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "unknown placement directive.*")
}

func (s *environBrokerSuite) TestStartInstanceWithZoneConstraintsAvailable(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	target := lxdtesting.NewMockInstanceServer(ctrl)
	tExp := target.EXPECT()
	serverRet := &api.Server{}
	image := &api.Image{Filename: "container-image"}

	tExp.GetServer().Return(serverRet, lxdtesting.ETag, nil)
	tExp.GetImageAlias("juju/ubuntu@24.04/amd64").Return(&api.ImageAliasesEntry{}, lxdtesting.ETag, nil)
	tExp.GetImage("").Return(image, lxdtesting.ETag, nil)

	jujuTarget, err := containerlxd.NewServer(target)
	c.Assert(err, tc.ErrorIsNil)

	members := []api.ClusterMember{
		{
			ServerName: "node01",
			Status:     "ONLINE",
		},
		{
			ServerName: "node02",
			Status:     "ONLINE",
		},
	}

	createOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createOp.EXPECT().Wait().Return(nil)
	createOp.EXPECT().GetTarget().Return(&api.Operation{StatusCode: api.Success}, nil)

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
		sExp.IsClustered().Return(true),
		sExp.UseTargetServer(gomock.Any(), "node01").Return(jujuTarget, nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		sExp.HostArch().Return(arch.AMD64),
	)

	// CreateContainerFromSpec is tested in container/lxd.
	// we don't bother with detailed parameter assertions here.
	tExp.CreateInstanceFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(createOp, nil)
	tExp.UpdateInstanceState(gomock.Any(), gomock.Any(), "").Return(startOp, nil)
	tExp.GetInstance(gomock.Any()).Return(&api.Instance{Type: "container", Location: "node01"}, lxdtesting.ETag, nil)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.AvailabilityZone = "node01"

	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithZoneConstraintsNotPresent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "ONLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.AvailabilityZone = "node03"

	_, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `availability zone "node03" not valid`)
}

func (s *environBrokerSuite) TestStartInstanceWithZoneConstraintsNotAvailable(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "OFFLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.HostArch().Return(arch.AMD64),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)

	args := s.GetStartInstanceArgs(c)
	args.AvailabilityZone = "node01"

	_, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `availability zone "node01" is "OFFLINE"`)
}

func (s *environBrokerSuite) TestStartInstanceWithConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// Check that the constraints were passed through to spec.Config.
	check := func(spec containerlxd.ContainerSpec) bool {
		cfg := spec.Config
		if cfg["limits.cpu"] != "2" {
			return false
		}
		if cfg["limits.memory"] != "2048MiB" {
			return false
		}
		return spec.InstanceType == "t2.micro"
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "node01"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	args := s.GetStartInstanceArgs(c)
	cores := uint64(2)
	mem := uint64(2048)
	it := "t2.micro"
	args.Constraints = constraints.Value{
		CpuCores:     &cores,
		Mem:          &mem,
		InstanceType: &it,
	}

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithConstraintsAndVirtType(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// Check that the constraints were passed through to spec.Config.
	check := func(spec containerlxd.ContainerSpec) bool {
		cfg := spec.Config
		if cfg["limits.cpu"] != "2" {
			return false
		}
		if cfg["limits.memory"] != "2048MiB" {
			return false
		}
		return spec.InstanceType == "t2.micro" && spec.VirtType == instance.InstanceTypeVM
	}

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)
	exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeVM, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil)
	exp.ServerVersion().Return("3.10.0")
	exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil)
	exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
		Instance: api.Instance{Location: "node01"},
	}, nil)
	exp.HostArch().Return(arch.AMD64)

	args := s.GetStartInstanceArgs(c)
	cores := uint64(2)
	mem := uint64(2048)
	it := "t2.micro"
	virtType := string(api.InstanceTypeVM)
	args.Constraints = constraints.Value{
		CpuCores:     &cores,
		Mem:          &mem,
		InstanceType: &it,
		VirtType:     &virtType,
	}

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceWithCharmLXDProfile(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// Check that the lxd profile name was passed through to spec.Config.
	check := func(spec containerlxd.ContainerSpec) bool {
		profiles := spec.Profiles
		if len(profiles) != 3 {
			return false
		}
		if profiles[0] != "default" {
			return false
		}
		if profiles[1] != "juju-" {
			return false
		}
		return profiles[2] == "juju-model-test-0"
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{
			Instance: api.Instance{Location: "node01"},
		}, nil),
		exp.HostArch().Return(arch.AMD64),
	)

	args := s.GetStartInstanceArgs(c)
	args.CharmLXDProfiles = []string{"juju-model-test-0"}

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	res, err := env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(*res.Hardware.AvailabilityZone, tc.DeepEquals, "node01")
}

func (s *environBrokerSuite) TestStartInstanceNoTools(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.PPC64EL)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	_, err := env.StartInstance(c.Context(), s.GetStartInstanceArgs(c))
	c.Assert(err, tc.ErrorMatches, "no matching agent binaries available")
}

func (s *environBrokerSuite) TestStartInstanceInvalidCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HostArch().Return(arch.AMD64),
		exp.FindImage(gomock.Any(), corebase.MakeDefaultBase("ubuntu", "24.04"), arch.AMD64, instance.InstanceTypeContainer, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.ServerVersion().Return("3.10.0"),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(gomock.Any()).Return(&containerlxd.Container{}, fmt.Errorf("not authorized")),
	)

	invalidator.EXPECT().InvalidateCredentials(gomock.Any(), environs.CredentialInvalidReason("cloud denied access: not authorized")).Return(nil)

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	_, err := env.StartInstance(c.Context(), s.GetStartInstanceArgs(c))
	c.Assert(err, tc.ErrorMatches, "not authorized")
}

func (s *environBrokerSuite) TestStopInstances(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	svr.EXPECT().RemoveContainers([]string{"juju-f75cba-1", "juju-f75cba-2"})

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	err := env.StopInstances(c.Context(), "juju-f75cba-1", "juju-f75cba-2", "not-in-namespace-so-ignored")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStopInstancesInvalidCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)
	invalidator.EXPECT().InvalidateCredentials(gomock.Any(), environs.CredentialInvalidReason("cloud denied access: not authorized")).Return(nil)

	svr.EXPECT().RemoveContainers([]string{"juju-f75cba-1", "juju-f75cba-2"}).Return(fmt.Errorf("not authorized"))

	env := s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator)
	err := env.StopInstances(c.Context(), "juju-f75cba-1", "juju-f75cba-2", "not-in-namespace-so-ignored")
	c.Assert(err, tc.ErrorMatches, "not authorized")
}

func (s *environBrokerSuite) TestImageSourcesDefault(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	sources, err := lxd.GetImageSources(c, s.NewEnviron(c, svr, nil, environscloudspec.CloudSpec{}, invalidator))
	c.Assert(err, tc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://cloud-images.ubuntu.com/releases/",
		"https://images.linuxcontainers.org",
	})
}

func (s *environBrokerSuite) TestImageMetadataURL(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-metadata-url": "https://my-test.com/images/",
	}, environscloudspec.CloudSpec{}, invalidator)

	sources, err := lxd.GetImageSources(c, env)
	c.Assert(err, tc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://my-test.com/images/",
		"https://cloud-images.ubuntu.com/releases/",
		"https://images.linuxcontainers.org",
	})
}

func (s *environBrokerSuite) TestImageMetadataURLEnsuresHTTPS(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	// HTTP should be converted to HTTPS.
	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-metadata-url": "http://my-test.com/images/",
	}, environscloudspec.CloudSpec{}, invalidator)

	sources, err := lxd.GetImageSources(c, env)
	c.Assert(err, tc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://my-test.com/images/",
		"https://cloud-images.ubuntu.com/releases/",
		"https://images.linuxcontainers.org",
	})
}

func (s *environBrokerSuite) TestImageStreamReleased(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-stream": "released",
	}, environscloudspec.CloudSpec{}, invalidator)

	sources, err := lxd.GetImageSources(c, env)
	c.Assert(err, tc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://cloud-images.ubuntu.com/releases/",
		"https://images.linuxcontainers.org",
	})
}

func (s *environBrokerSuite) TestImageStreamDaily(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-stream": "daily",
	}, environscloudspec.CloudSpec{}, invalidator)

	sources, err := lxd.GetImageSources(c, env)
	c.Assert(err, tc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://cloud-images.ubuntu.com/daily/",
		"https://images.linuxcontainers.org",
	})
}

func (s *environBrokerSuite) checkSources(c *tc.C, sources []containerlxd.ServerSpec, expectedURLs []string) {
	var sourceURLs []string
	for _, source := range sources {
		sourceURLs = append(sourceURLs, source.Host)
	}
	c.Check(sourceURLs, tc.DeepEquals, expectedURLs)
}
