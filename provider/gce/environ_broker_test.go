// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"errors"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/storage"
)

type environBrokerSuite struct {
	gce.BaseSuite

	spec *instances.InstanceSpec
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	instanceType := instances.InstanceType{
		Name:     "n1-standard-1",
		Arch:     arch.AMD64,
		CpuCores: 2,
		Mem:      3750,
		RootDisk: 15 * 1024,
		VirtType: ptr("kvm"),
	}
	s.spec = &instances.InstanceSpec{
		InstanceType: instanceType,
		Image: instances.Image{
			Id:       "ubuntu-2204-jammy-v20141212",
			Arch:     arch.AMD64,
			VirtType: "kvm",
		},
	}
}

func (s *environBrokerSuite) expectImageMetadata() {
	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*compute.MachineType{{
		Id:           0,
		Name:         "n1-standard-1",
		GuestCpus:    int64(s.spec.InstanceType.CpuCores),
		MemoryMb:     int64(s.spec.InstanceType.Mem),
		Architecture: s.spec.InstanceType.Arch,
	}}, nil)
	s.StartInstArgs.ImageMetadata = []*imagemetadata.ImageMetadata{{
		Id:   "ubuntu-220-jammy-v20141212",
		Arch: s.spec.InstanceType.Arch,
	}}
}

func (s *environBrokerSuite) startInstanceArg(c *gc.C, prefix string) *compute.Instance {
	instName := fmt.Sprintf("%s0", prefix)
	userData, err := providerinit.ComposeUserData(s.StartInstArgs.InstanceConfig, nil, gce.GCERenderer{})
	c.Assert(err, jc.ErrorIsNil)

	return &compute.Instance{
		Name:        instName,
		Zone:        "home-zone",
		MachineType: "zones/home-zone/machineTypes/n1-standard-1",
		Disks: []*compute.AttachedDisk{{
			AutoDelete: true,
			Boot:       true,
			Mode:       "READ_WRITE",
			Type:       "PERSISTENT",
			InitializeParams: &compute.AttachedDiskInitializeParams{
				DiskSizeGb:  10,
				SourceImage: "projects/ubuntu-os-cloud/global/images/ubuntu-220-jammy-v20141212",
			},
		}},
		NetworkInterfaces: []*compute.NetworkInterface{{
			Network: "global/networks/default",
			AccessConfigs: []*compute.AccessConfig{{
				Name: "ExternalNAT",
				Type: "ONE_TO_ONE_NAT",
			}},
		}},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{{
				Key:   "juju-controller-uuid",
				Value: ptr(s.ControllerUUID),
			}, {
				Key:   "juju-is-controller",
				Value: ptr("true"),
			}, {
				Key:   "user-data",
				Value: ptr(string(userData)),
			}, {
				Key:   "user-data-encoding",
				Value: ptr("base64"),
			}},
		},
		Tags: &compute.Tags{Items: []string{"juju-" + s.ModelUUID, instName}},
		ServiceAccounts: []*compute.ServiceAccount{{
			Email: "fred@google.com",
		}},
	}
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)

	s.expectImageMetadata()
	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@google.com", nil)

	instArg := s.startInstanceArg(c, s.Prefix(env))
	instResult := *instArg
	instResult.Zone = "path/to/home-zone"
	instResult.Disks = []*compute.AttachedDisk{{
		DiskSizeGb: int64(s.spec.InstanceType.RootDisk / 1024),
	}}

	s.MockService.EXPECT().AddInstance(gomock.Any(), instArg).Return(&instResult, nil)

	s.StartInstArgs.AvailabilityZone = "home-zone"
	result, err := env.StartInstance(s.CallCtx, s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.GoogleInstance(c, result.Instance), jc.DeepEquals, &instResult)

	hwc := &instance.HardwareCharacteristics{
		Arch:             &s.spec.InstanceType.Arch,
		Mem:              &s.spec.InstanceType.Mem,
		CpuCores:         &s.spec.InstanceType.CpuCores,
		CpuPower:         s.spec.InstanceType.CpuPower,
		RootDisk:         ptr(s.spec.InstanceType.RootDisk),
		AvailabilityZone: ptr("home-zone"),
	}
	c.Check(result.Hardware, jc.DeepEquals, hwc)
}

func (s *environBrokerSuite) TestStartInstanceAvailabilityZoneIndependentError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, errors.New("blargh"))

	_, err := env.StartInstance(s.CallCtx, s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, "blargh")
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), jc.IsTrue)
}

func (s *environBrokerSuite) TestStartInstanceVolumeAvailabilityZone(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)

	s.expectImageMetadata()
	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@google.com", nil)

	instArg := s.startInstanceArg(c, s.Prefix(env))
	instResult := *instArg
	instResult.Zone = "path/to/home-zone"
	instResult.Disks = []*compute.AttachedDisk{{
		DiskSizeGb: int64(s.spec.InstanceType.RootDisk / 1024),
	}}

	s.MockService.EXPECT().AddInstance(gomock.Any(), instArg).Return(&instResult, nil)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	derivedZones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(derivedZones, gc.HasLen, 1)
	s.StartInstArgs.AvailabilityZone = derivedZones[0]

	result, err := env.StartInstance(s.CallCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*result.Hardware.AvailabilityZone, gc.Equals, derivedZones[0])
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.AgentVersion(), gc.Not(gc.Equals), version.Binary{})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectImageMetadata()

	spec, err := gce.BuildInstanceSpec(env, s.CallCtx, s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec.InstanceType, jc.DeepEquals, instances.InstanceType{
		Id:         "0",
		Name:       "n1-standard-1",
		Arch:       "amd64",
		CpuCores:   2,
		Mem:        3750,
		Networking: instances.InstanceTypeNetworking{},
		VirtType:   ptr("kvm"),
	})
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ic := &instances.InstanceConstraint{
		Region:      "home",
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Arch:        arch.AMD64,
		Constraints: s.StartInstArgs.Constraints,
	}
	imageMetadata := []*imagemetadata.ImageMetadata{{
		Id:         "ubuntu-2204-jammy-v20141212",
		Arch:       "amd64",
		Version:    "22.04",
		RegionName: "us-central1",
		Endpoint:   "https://www.googleapis.com",
		Stream:     "<stream>",
		VirtType:   "kvm",
	}}
	spec, err := gce.FindInstanceSpec(env, ic, imageMetadata, []instances.InstanceType{s.spec.InstanceType})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec, jc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestGetMetadataUbuntu(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)

	userData, err := providerinit.ComposeUserData(s.StartInstArgs.InstanceConfig, nil, gce.GCERenderer{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, jc.DeepEquals, map[string]string{
		tags.JujuIsController: "true",
		tags.JujuController:   s.ControllerUUID,
		"user-data":           string(userData),
		"user-data-encoding":  "base64",
	})
}

func (s *environBrokerSuite) TestGetMetadataOSNotSupported(c *gc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.GenericLinux)

	c.Assert(metadata, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot pack metadata for os GenericLinux on the gce provider")
}

var getDisksTests = []struct {
	osname string
	error  error
}{
	{"ubuntu", nil},
	{"suse", errors.New("os Suse is not supported on the gce provider")},
}

func (s *environBrokerSuite) TestGetDisks(c *gc.C) {
	for _, test := range getDisksTests {
		os := ostype.OSTypeForName(test.osname)
		diskSpecs, err := gce.GetDisks("image-url", s.StartInstArgs.Constraints, os)
		if test.error != nil {
			c.Assert(err, gc.Equals, err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(diskSpecs, gc.HasLen, 1)
			diskSpec := diskSpecs[0]
			c.Assert(diskSpec.InitializeParams, gc.NotNil)
			c.Check(diskSpec.InitializeParams.DiskSizeGb, gc.Equals, int64(10))
			c.Check(diskSpec.InitializeParams.SourceImage, gc.Equals, "image-url")
		}
	}
}

func (s *environBrokerSuite) TestGetHardwareCharacteristics(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	hwc := gce.GetHardwareCharacteristics(env, s.spec, s.NewEnvironInstance(c, env, "inst-0"))

	c.Assert(hwc, gc.NotNil)
	c.Check(*hwc.Arch, gc.Equals, "amd64")
	c.Check(*hwc.AvailabilityZone, gc.Equals, "home-zone")
	c.Check(*hwc.CpuCores, gc.Equals, uint64(2))
	c.Check(*hwc.Mem, gc.Equals, uint64(3750))
	c.Check(*hwc.RootDisk, gc.Equals, uint64(15360))
}

func (s *environBrokerSuite) TestAllRunningInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	insts, err := env.AllRunningInstances(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insts, jc.DeepEquals, []instances.Instance{s.NewEnvironInstance(c, env, "inst-0")})
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0").Return(nil)

	err := env.StopInstances(s.CallCtx, "inst-0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStopInstancesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0").Return(gce.InvalidCredentialError)

	err := env.StopInstances(s.CallCtx, "inst-0")
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}
