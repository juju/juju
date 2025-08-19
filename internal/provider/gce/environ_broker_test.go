// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"errors"
	"testing"

	"github.com/juju/tc"

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

func TestEnvironBrokerSuite(t *testing.T) {
	tc.Run(t, &environBrokerSuite{})
}

func (s *environBrokerSuite) SetUpTest(c *tc.C) {
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

func (s *environBrokerSuite) TestStartInstance(c *tc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	result, err := s.Env.StartInstance(c.Context(), s.StartInstArgs)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Instance, tc.DeepEquals, s.Instance)
	c.Check(result.Hardware, tc.DeepEquals, s.hardware)
}

func (s *environBrokerSuite) TestStartInstanceAvailabilityZoneIndependentError(c *tc.C) {
	s.FakeEnviron.Err = errors.New("blargh")

	_, err := s.Env.StartInstance(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, "blargh")
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestStartInstanceVolumeAvailabilityZone(c *tc.C) {
	s.FakeEnviron.Spec = s.spec
	s.FakeEnviron.Inst = s.BaseInstance
	s.FakeEnviron.Hwc = s.hardware

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	derivedZones, err := s.Env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(derivedZones, tc.HasLen, 1)
	s.StartInstArgs.AvailabilityZone = derivedZones[0]

	result, err := s.Env.StartInstance(c.Context(), s.StartInstArgs)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*result.Hardware.AvailabilityZone, tc.Equals, derivedZones[0])
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *tc.C) {
	err := gce.FinishInstanceConfig(s.Env, s.StartInstArgs, s.spec)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.AgentVersion(), tc.Not(tc.Equals), semversion.Binary{})
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *tc.C) {
	s.FakeEnviron.Spec = s.spec

	spec, err := gce.BuildInstanceSpec(s.Env, c.Context(), s.StartInstArgs)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(spec.InstanceType, tc.DeepEquals, s.InstanceType)
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *tc.C) {
	spec, err := gce.FindInstanceSpec(s.Env, s.ic, s.imageMetadata, []instances.InstanceType{s.InstanceType})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(spec, tc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestNewRawInstance(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	inst, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(inst, tc.DeepEquals, s.BaseInstance)
}

func (s *environBrokerSuite) TestNewRawInstanceNoPublicIP(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	public := false
	s.StartInstArgs.Constraints.AllocatePublicIP = &public

	inst, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)

	nics := inst.NetworkInterfaces()
	c.Assert(nics, tc.HasLen, 1)
	c.Assert(nics[0].AccessConfigs, tc.HasLen, 0)
}

func (s *environBrokerSuite) TestNewRawInstanceZoneInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestNewRawInstanceZoneSpecificError(c *tc.C) {
	s.FakeConn.Err = errors.New("blargh")

	_, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorMatches, "blargh")
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
}

func (s *environBrokerSuite) TestGetMetadataUbuntu(c *tc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.Ubuntu)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadata, tc.DeepEquals, s.UbuntuMetadata)

}

func (s *environBrokerSuite) TestGetMetadataOSNotSupported(c *tc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.GenericLinux)

	c.Assert(metadata, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cannot pack metadata for os GenericLinux on the gce provider")
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

func (s *environBrokerSuite) TestGetDisks(c *tc.C) {
	for _, test := range getDisksTests {
		os := ostype.OSTypeForName(test.osname)
		diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, os, "32f7d570-5bac-4b72-b169-250c24a94b2b", test.basePath)
		if test.error != nil {
			c.Assert(err, tc.Equals, err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(diskSpecs, tc.HasLen, 1)

			diskSpec := diskSpecs[0]

			switch os {
			case ostype.Ubuntu:
				c.Check(diskSpec.SizeHintGB, tc.Equals, uint64(8))
			case ostype.Windows:
				c.Check(diskSpec.SizeHintGB, tc.Equals, uint64(40))
			default:
				c.Check(diskSpec.SizeHintGB, tc.Equals, uint64(8))
			}
			c.Check(diskSpec.ImageURL, tc.Equals, test.basePath+s.spec.Image.Id)
		}
	}

	diskSpecs, err := gce.GetDisks(s.spec, s.StartInstArgs.Constraints, ostype.Ubuntu, "32f7d570-5bac-4b72-b169-250c24a94b2b", gce.UbuntuDailyImageBasePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(diskSpecs, tc.HasLen, 1)
	spec := diskSpecs[0]
	c.Assert(spec.ImageURL, tc.Equals, gce.UbuntuDailyImageBasePath+s.spec.Image.Id)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfig(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "released"})
	result, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.NotNil)
	//c.Check(c.GetTestLog(), tc.Contains, gce.UbuntuImageBasePath)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfigToDaily(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "daily"})
	result, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.NotNil)
	//c.Check(c.GetTestLog(), tc.Contains, gce.UbuntuDailyImageBasePath)
}

func (s *environBrokerSuite) TestSettingImageStreamsViaConfigToPro(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{"image-stream": "pro"})
	result, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.NotNil)
	//c.Check(c.GetTestLog(), tc.Contains, gce.UbuntuProImageBasePath)
}

func (s *environBrokerSuite) TestSettingBaseImagePathOverwritesImageStreams(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	s.UpdateConfig(c, map[string]interface{}{
		"image-stream":    "daily",
		"base-image-path": "/opt/custom-builds/",
	})
	result, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.NotNil)
	//c.Check(c.GetTestLog(), tc.Contains, "/opt/custom-builds/")
}

func (s *environBrokerSuite) TestSettingServiceAccountFromClientEmail(c *tc.C) {
	s.FakeConn.Inst = s.BaseInstance
	result, err := gce.NewRawInstance(s.Env, c.Context(), s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.InstanceSummary.ServiceAccount, tc.Equals, "fred@foo.com")
}

func (s *environBrokerSuite) TestGetHardwareCharacteristics(c *tc.C) {
	hwc := gce.GetHardwareCharacteristics(s.Env, s.spec, s.Instance)

	c.Assert(hwc, tc.NotNil)
	c.Check(*hwc.Arch, tc.Equals, "amd64")
	c.Check(*hwc.AvailabilityZone, tc.Equals, "home-zone")
	c.Check(*hwc.CpuCores, tc.Equals, uint64(1))
	c.Check(*hwc.CpuPower, tc.Equals, uint64(275))
	c.Check(*hwc.Mem, tc.Equals, uint64(3750))
	c.Check(*hwc.RootDisk, tc.Equals, uint64(15360))
}

func (s *environBrokerSuite) TestAllRunningInstances(c *tc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	insts, err := s.Env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insts, tc.DeepEquals, []instances.Instance{s.Instance})
}

func (s *environBrokerSuite) TestStopInstances(c *tc.C) {
	err := s.Env.StopInstances(c.Context(), s.Instance.Id())
	c.Assert(err, tc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("RemoveInstances")
	c.Check(called, tc.Equals, true)
	c.Check(calls, tc.HasLen, 1)
	c.Check(calls[0].Prefix, tc.Equals, s.Prefix())
	c.Check(calls[0].IDs, tc.DeepEquals, []string{"spam"})
}

func (s *environBrokerSuite) TestStopInstancesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	err := s.Env.StopInstances(c.Context(), s.Instance.Id())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}
