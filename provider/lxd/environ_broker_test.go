// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/testing"
)

type environBrokerSuite struct {
	gce.BaseSuite

	hardware      *instance.HardwareCharacteristics
	spec          *instances.InstanceSpec
	ic            *instances.InstanceConstraint
	imageMetadata []*imagemetadata.ImageMetadata
	resolveInfo   *simplestreams.ResolveInfo
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	mem := uint64(3750)
	amd64 := arch.AMD64
	cpuCores := uint64(1)
	rootDiskMB := uint64(5)
	zoneName := "home-zone"

	s.hardware = &instance.HardwareCharacteristics{
		Arch:             &amd64,
		Mem:              &mem,
		CpuCores:         &cpuCores,
		CpuPower:         instances.CpuPower(275),
		RootDisk:         &rootDiskMB,
		AvailabilityZone: &zoneName,
	}
	s.spec = &instances.InstanceSpec{
		InstanceType: s.InstanceType,
		Image: instances.Image{
			Id:       "ubuntu-1404-trusty-v20141212",
			Arch:     amd64,
			VirtType: "kvm",
		},
	}
	s.ic = &instances.InstanceConstraint{
		Region:      "home",
		Series:      "trusty",
		Arches:      []string{amd64},
		Constraints: s.StartInstArgs.Constraints,
	}
	s.imageMetadata = []*imagemetadata.ImageMetadata{{
		Id:         "ubuntu-1404-trusty-v20141212",
		Arch:       "amd64",
		Version:    "14.04",
		RegionName: "us-central1",
		Endpoint:   "https://www.googleapis.com",
		Stream:     "<stream>",
		VirtType:   "kvm",
	}}
	s.resolveInfo = &simplestreams.ResolveInfo{
		Source:    "",
		Signed:    true,
		IndexURL:  "",
		MirrorURL: "",
	}
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	result, err := s.Env.StartInstance(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.hardware)
}

func (s *environBrokerSuite) TestStartInstanceOpensAPIPort(c *gc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	// Get the API port from the fake environment config used to
	// "bootstrap".
	envConfig := testing.FakeConfig()
	apiPort, ok := envConfig["api-port"].(int)
	c.Assert(ok, jc.IsTrue)
	c.Assert(apiPort, gc.Not(gc.Equals), 0)

	// When StateServingInfo is not nil, verify OpenPorts was called
	// for the API port.
	s.StartInstArgs.InstanceConfig.StateServingInfo = &params.StateServingInfo{
		APIPort: apiPort,
	}

	result, err := s.Env.StartInstance(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.hardware)

	called, calls := s.FakeConn.WasCalled("OpenPorts")
	c.Check(called, gc.Equals, true)
	c.Check(calls, gc.HasLen, 1)
	c.Check(calls[0].FirewallName, gc.Equals, gce.GlobalFirewallName(s.Env))
	expectPorts := []network.PortRange{{
		FromPort: apiPort,
		ToPort:   apiPort,
		Protocol: "tcp",
	}}
	c.Check(calls[0].PortRanges, jc.DeepEquals, expectPorts)
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *gc.C) {
	err := gce.FinishInstanceConfig(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.Tools, gc.NotNil)
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *gc.C) {
	s.FakeEnviron.Spec = s.spec

	spec, err := gce.BuildInstanceSpec(s.Env, s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec.InstanceType, gc.DeepEquals, s.InstanceType)
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *gc.C) {
	s.FakeImages.Metadata = s.imageMetadata
	s.FakeImages.ResolveInfo = s.resolveInfo

	spec, err := gce.FindInstanceSpec(s.Env, s.Env.Config().ImageStream(), s.ic)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec, gc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestNewRawInstance(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	inst, err := gce.NewRawInstance(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(inst, gc.DeepEquals, s.BaseInstance)
}

func (s *environBrokerSuite) TestGetMetadata(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, s.Metadata)
}

func (s *environBrokerSuite) TestGetDisks(c *gc.C) {
	diskSpecs := gce.GetDisks(s.spec, s.StartInstArgs.Constraints)

	c.Assert(diskSpecs, gc.HasLen, 1)

	diskSpec := diskSpecs[0]

	c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(8))
	c.Check(diskSpec.ImageURL, gc.Equals, "projects/ubuntu-os-cloud/global/images/ubuntu-1404-trusty-v20141212")
}

func (s *environBrokerSuite) TestGetHardwareCharacteristics(c *gc.C) {
	hwc := gce.GetHardwareCharacteristics(s.Env, s.spec, s.Instance)

	c.Assert(hwc, gc.NotNil)
	c.Check(*hwc.Arch, gc.Equals, "amd64")
	c.Check(*hwc.AvailabilityZone, gc.Equals, "home-zone")
	c.Check(*hwc.CpuCores, gc.Equals, uint64(1))
	c.Check(*hwc.CpuPower, gc.Equals, uint64(275))
	c.Check(*hwc.Mem, gc.Equals, uint64(3750))
	c.Check(*hwc.RootDisk, gc.Equals, uint64(15360))
}

func (s *environBrokerSuite) TestAllInstances(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}

	insts, err := s.Env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insts, jc.DeepEquals, []instance.Instance{s.Instance})
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.Env.StopInstances(s.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("RemoveInstances")
	c.Check(called, gc.Equals, true)
	c.Check(calls, gc.HasLen, 1)
	c.Check(calls[0].Prefix, gc.Equals, "juju-2d02eeac-9dbb-11e4-89d3-123b93f75cba-machine-")
	c.Check(calls[0].IDs, gc.DeepEquals, []string{"spam"})
}
