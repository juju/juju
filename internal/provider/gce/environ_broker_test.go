// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/storage"
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
			Id:       "ubuntu-2204-jammy-v20141212",
			Arch:     amd64,
			VirtType: "kvm",
		},
	}
	s.ic = &instances.InstanceConstraint{
		Region:      "home",
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Arch:        amd64,
		Constraints: s.StartInstArgs.Constraints,
	}
	s.imageMetadata = []*imagemetadata.ImageMetadata{{
		Id:         "ubuntu-2204-jammy-v20141212",
		Arch:       "amd64",
		Version:    "22.04",
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

	// NOTE(achilleasa): at least one zone is required so that any tests
	// that trigger a call to InstanceTypes can obtain a non-empty instance
	// list.
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	result, err := s.Env.StartInstance(context.Background(), s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, jc.DeepEquals, s.Instance)
	c.Check(result.Hardware, jc.DeepEquals, s.hardware)
}

func (s *environBrokerSuite) TestStartInstanceAvailabilityZoneIndependentError(c *gc.C) {
	s.FakeEnviron.Err = errors.New("blargh")

	_, err := s.Env.StartInstance(context.Background(), s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, "blargh")
	c.Assert(err, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestStartInstanceVolumeAvailabilityZone(c *gc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	derivedZones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(derivedZones, gc.HasLen, 1)
	s.StartInstArgs.AvailabilityZone = derivedZones[0]

	result, err := s.Env.StartInstance(context.Background(), s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*result.Hardware.AvailabilityZone, gc.Equals, derivedZones[0])
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *gc.C) {
	err := gce.FinishInstanceConfig(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.AgentVersion(), gc.Not(gc.Equals), semversion.Binary{})
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *gc.C) {
	s.FakeEnviron.Spec = s.spec

	spec, err := gce.BuildInstanceSpec(s.Env, context.Background(), s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec.InstanceType, jc.DeepEquals, s.InstanceType)
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *gc.C) {
	spec, err := gce.FindInstanceSpec(s.Env, s.ic, s.imageMetadata, []instances.InstanceType{s.InstanceType})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec, jc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestNewRawInstance(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	inst, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(inst, jc.DeepEquals, s.BaseInstance)
}

func (s *environBrokerSuite) TestNewRawInstanceNoPublicIP(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	public := false
	s.StartInstArgs.Constraints.AllocatePublicIP = &public

	inst, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)

	nics := inst.NetworkInterfaces()
	c.Assert(nics, gc.HasLen, 1)
	c.Assert(nics[0].AccessConfigs, gc.HasLen, 0)
}

func (s *environBrokerSuite) TestNewRawInstanceZoneInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
	c.Assert(err, gc.Not(jc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestNewRawInstanceZoneSpecificError(c *gc.C) {
	s.FakeConn.Err = errors.New("blargh")

	_, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, gc.ErrorMatches, "blargh")
	c.Assert(err, gc.Not(jc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestGetMetadataUbuntu(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.Ubuntu)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, jc.DeepEquals, s.UbuntuMetadata)

}

func (s *environBrokerSuite) TestGetMetadataOSNotSupported(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.GenericLinux)

	c.Assert(metadata, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot pack metadata for os GenericLinux on the gce provider")
}

var getDisksTests = []struct {
	osname   string
	basePath string
	error    error
}{
	{"ubuntu", gce.UbuntuImageBasePath, nil},
	{"ubuntu", "/tmp/", nil}, // --config base-image-path=/tmp/
	{"suse", "", errors.New("os Suse is not supported on the gce provider")},
}

func (s *environBrokerSuite) TestGetDisks(c *gc.C) {
	for _, test := range getDisksTests {
		os := ostype.OSTypeForName(test.osname)
		diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, os, "32f7d570-5bac-4b72-b169-250c24a94b2b", test.basePath)
		if test.error != nil {
			c.Assert(err, gc.Equals, err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(diskSpecs, gc.HasLen, 1)

			diskSpec := diskSpecs[0]

			switch os {
			case ostype.Ubuntu:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(8))
			case ostype.Windows:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(40))
			default:
				c.Check(diskSpec.SizeHintGB, gc.Equals, uint64(8))
			}
			c.Check(diskSpec.ImageURL, gc.Equals, test.basePath+s.spec.Image.Id)
		}
	}

	diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, ostype.Ubuntu, "32f7d570-5bac-4b72-b169-250c24a94b2b", gce.UbuntuDailyImageBasePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(diskSpecs, gc.HasLen, 1)
	spec := diskSpecs[0]
	c.Assert(spec.ImageURL, gc.Equals, gce.UbuntuDailyImageBasePath+s.spec.Image.Id)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfig(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "released"})
	result, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.NotNil)
	c.Check(c.GetTestLog(), jc.Contains, gce.UbuntuImageBasePath)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfigToDaily(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "daily"})
	result, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.NotNil)
	c.Check(c.GetTestLog(), jc.Contains, gce.UbuntuDailyImageBasePath)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfigToPro(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "pro"})
	result, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.NotNil)
	c.Check(c.GetTestLog(), jc.Contains, gce.UbuntuProImageBasePath)
}

func (s *environBrokerSuite) TestSettingBaseImagePathOverwritesImageStreams(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{
		"image-stream":    "daily",
		"base-image-path": "/opt/custom-builds/",
	})
	result, err := gce.NewRawInstance(s.Env, context.Background(), s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.NotNil)
	c.Check(c.GetTestLog(), jc.Contains, "/opt/custom-builds/")
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

func (s *environBrokerSuite) TestAllRunningInstances(c *gc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	insts, err := s.Env.AllRunningInstances(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insts, jc.DeepEquals, []instances.Instance{s.Instance})
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.Env.StopInstances(context.Background(), s.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("RemoveInstances")
	c.Check(called, gc.Equals, true)
	c.Check(calls, gc.HasLen, 1)
	c.Check(calls[0].Prefix, gc.Equals, s.Prefix())
	c.Check(calls[0].IDs, jc.DeepEquals, []string{"spam"})
}

func (s *environBrokerSuite) TestStopInstancesInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.StopInstances(context.Background(), s.Instance.Id())
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}
