// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce"
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
	c.Check(result.Instance, jc.DeepEquals, s.Instance)
	c.Check(result.Hardware, jc.DeepEquals, s.hardware)
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *gc.C) {
	err := gce.FinishInstanceConfig(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.AgentVersion(), gc.Not(gc.Equals), version.Binary{})
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *gc.C) {
	s.FakeEnviron.Spec = s.spec

	spec, err := gce.BuildInstanceSpec(s.Env, s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec.InstanceType, jc.DeepEquals, s.InstanceType)
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *gc.C) {
	spec, err := gce.FindInstanceSpec(s.Env, s.ic, s.imageMetadata)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec, jc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestNewRawInstance(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	inst, err := gce.NewRawInstance(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(inst, jc.DeepEquals, s.BaseInstance)
}

func (s *environBrokerSuite) TestGetMetadataUbuntu(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, jujuos.Ubuntu)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, jc.DeepEquals, s.UbuntuMetadata)

}

func (s *environBrokerSuite) TestGetMetadataWindows(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, jujuos.Windows)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata["windows-startup-script-ps1"], gc.Equals, s.WindowsMetadata["windows-startup-script-ps1"])
	c.Check(metadata["sysprep-specialize-script-ps1"], gc.Matches, s.WindowsMetadata["sysprep-specialize-script-ps1"])
}

func (s *environBrokerSuite) TestGetMetadataOSNotSupported(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, jujuos.GenericLinux)

	c.Assert(metadata, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot pack metadata for os GenericLinux on the gce provider")
}

var getDisksTests = []struct {
	Series   string
	basePath string
	error    error
}{
	{"trusty", gce.UbuntuImageBasePath, nil},
	{"win2012r2", gce.WindowsImageBasePath, nil},
	{"arch", "", errors.New("os Arch is not supported on the gce provider")},
}

func (s *environBrokerSuite) TestGetDisks(c *gc.C) {
	for _, test := range getDisksTests {
		diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, test.Series, "32f7d570-5bac-4b72-b169-250c24a94b2b", false)
		if test.error != nil {
			c.Assert(err, gc.Equals, err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(diskSpecs, gc.HasLen, 1)

			diskSpec := diskSpecs[0]

			os, err := series.GetOSFromSeries(test.Series)
			c.Assert(err, jc.ErrorIsNil)
			switch os {
			case jujuos.Ubuntu:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(8))
			case jujuos.Windows:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(40))
			default:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(8))
			}
			c.Check(diskSpec.ImageURL, gc.Equals, test.basePath+s.spec.Image.Id)
		}
	}

	diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, "trusty", "32f7d570-5bac-4b72-b169-250c24a94b2b", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(diskSpecs, gc.HasLen, 1)
	spec := diskSpecs[0]
	c.Assert(spec.ImageURL, gc.Equals, gce.UbuntuDailyImageBasePath+s.spec.Image.Id)
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
	c.Check(calls[0].Prefix, gc.Equals, s.Prefix())
	c.Check(calls[0].IDs, jc.DeepEquals, []string{"spam"})
}
