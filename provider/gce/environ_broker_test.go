// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
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

func (s *environBrokerSuite) TestFinishMachineConfig(c *gc.C) {
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
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp),
	}

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
}

func (s *environBrokerSuite) TestGetHardwareCharacteristics(c *gc.C) {
}

func (s *environBrokerSuite) TestAllInstances(c *gc.C) {
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
}
