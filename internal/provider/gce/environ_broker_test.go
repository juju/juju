// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/storage"
)

type environBrokerSuite struct {
	gce.BaseSuite

	spec *instances.InstanceSpec
}

func TestEnvironBrokerSuite(t *testing.T) {
	tc.Run(t, &environBrokerSuite{})
}

func (s *environBrokerSuite) SetUpTest(c *tc.C) {
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
	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-1"),
		GuestCpus:    ptr(int32(s.spec.InstanceType.CpuCores)),
		MemoryMb:     ptr(int32(s.spec.InstanceType.Mem)),
		Architecture: ptr(s.spec.InstanceType.Arch),
	}}, nil)
	s.StartInstArgs.ImageMetadata = []*imagemetadata.ImageMetadata{{
		Id:   "ubuntu-220-jammy-v20141212",
		Arch: s.spec.InstanceType.Arch,
	}}
}

func (s *environBrokerSuite) startInstanceArg(c *tc.C, prefix string) *computepb.Instance {
	instName := fmt.Sprintf("%s0", prefix)
	userData, err := providerinit.ComposeUserData(s.StartInstArgs.InstanceConfig, nil, gce.GCERenderer{})
	c.Assert(err, tc.ErrorIsNil)

	return &computepb.Instance{
		Name:        &instName,
		Zone:        ptr("home-zone"),
		MachineType: ptr("zones/home-zone/machineTypes/n1-standard-1"),
		Disks: []*computepb.AttachedDisk{{
			AutoDelete: ptr(true),
			Boot:       ptr(true),
			Mode:       ptr("READ_WRITE"),
			Type:       ptr("PERSISTENT"),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				DiskSizeGb:  ptr(int64(10)),
				SourceImage: ptr("projects/ubuntu-os-cloud/global/images/ubuntu-220-jammy-v20141212"),
			},
		}},
		NetworkInterfaces: []*computepb.NetworkInterface{{
			Network: ptr("global/networks/default"),
			AccessConfigs: []*computepb.AccessConfig{{
				Name: ptr("ExternalNAT"),
				Type: ptr("ONE_TO_ONE_NAT"),
			}},
		}},
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{{
				Key:   ptr("juju-controller-uuid"),
				Value: ptr(s.ControllerUUID),
			}, {
				Key:   ptr("juju-is-controller"),
				Value: ptr("true"),
			}, {
				Key:   ptr("user-data"),
				Value: ptr(string(userData)),
			}, {
				Key:   ptr("user-data-encoding"),
				Value: ptr("base64"),
			}},
		},
		Tags: &computepb.Tags{Items: []string{"juju-" + s.ModelUUID, instName}},
		ServiceAccounts: []*computepb.ServiceAccount{{
			Email: ptr("fred@google.com"),
		}},
	}
}

func (s *environBrokerSuite) TestStartInstance(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)

	s.expectImageMetadata()
	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@google.com", nil)

	instArg := s.startInstanceArg(c, s.Prefix(env))
	// Can't copy instArg as it contains a mutex.
	instResult := s.startInstanceArg(c, s.Prefix(env))
	instResult.Zone = ptr("path/to/home-zone")
	instResult.Disks = []*computepb.AttachedDisk{{
		DiskSizeGb: ptr(int64(s.spec.InstanceType.RootDisk / 1024)),
	}}

	s.MockService.EXPECT().AddInstance(gomock.Any(), instArg).Return(instResult, nil)

	s.StartInstArgs.AvailabilityZone = "home-zone"
	result, err := env.StartInstance(c.Context(), s.StartInstArgs)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.GoogleInstance(c, result.Instance), tc.DeepEquals, instResult)

	hwc := &instance.HardwareCharacteristics{
		Arch:             &s.spec.InstanceType.Arch,
		Mem:              &s.spec.InstanceType.Mem,
		CpuCores:         &s.spec.InstanceType.CpuCores,
		CpuPower:         s.spec.InstanceType.CpuPower,
		RootDisk:         ptr(s.spec.InstanceType.RootDisk),
		AvailabilityZone: ptr("home-zone"),
	}
	c.Check(result.Hardware, tc.DeepEquals, hwc)
}

func (s *environBrokerSuite) TestStartInstanceAvailabilityZoneIndependentError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, errors.New("blargh"))

	_, err := env.StartInstance(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, "blargh")
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), tc.IsTrue)
}

func (s *environBrokerSuite) TestStartInstanceVolumeAvailabilityZone(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)

	s.expectImageMetadata()
	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@google.com", nil)

	instArg := s.startInstanceArg(c, s.Prefix(env))
	// Can't copy instArg as it contains a mutex.
	instResult := s.startInstanceArg(c, s.Prefix(env))
	instResult.Zone = ptr("path/to/home-zone")
	instResult.Disks = []*computepb.AttachedDisk{{
		DiskSizeGb: ptr(int64(s.spec.InstanceType.RootDisk / 1024)),
	}}

	s.MockService.EXPECT().AddInstance(gomock.Any(), instArg).Return(instResult, nil)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	derivedZones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(derivedZones, tc.HasLen, 1)
	s.StartInstArgs.AvailabilityZone = derivedZones[0]

	result, err := env.StartInstance(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*result.Hardware.AvailabilityZone, tc.Equals, derivedZones[0])
}

func (s *environBrokerSuite) TestFinishInstanceConfig(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	err := gce.FinishInstanceConfig(env, s.StartInstArgs, s.spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.StartInstArgs.InstanceConfig.AgentVersion(), tc.Not(tc.Equals), semversion.Binary{})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *environBrokerSuite) TestBuildInstanceSpec(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectImageMetadata()

	spec, err := gce.BuildInstanceSpec(env, c.Context(), s.StartInstArgs)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(spec.InstanceType, tc.DeepEquals, instances.InstanceType{
		Id:         "0",
		Name:       "n1-standard-1",
		Arch:       "amd64",
		CpuCores:   2,
		Mem:        3750,
		Networking: instances.InstanceTypeNetworking{},
		VirtType:   ptr("kvm"),
	})
}

func (s *environBrokerSuite) TestFindInstanceSpec(c *tc.C) {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(spec, tc.DeepEquals, s.spec)
}

func (s *environBrokerSuite) TestGetMetadataUbuntu(c *tc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.Ubuntu)
	c.Assert(err, tc.ErrorIsNil)

	userData, err := providerinit.ComposeUserData(s.StartInstArgs.InstanceConfig, nil, gce.GCERenderer{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadata, tc.DeepEquals, map[string]string{
		tags.JujuIsController: "true",
		tags.JujuController:   s.ControllerUUID,
		"user-data":           string(userData),
		"user-data-encoding":  "base64",
	})
}

func (s *environBrokerSuite) TestGetMetadataOSNotSupported(c *tc.C) {
	metadata, err := gce.GetMetadata(s.StartInstArgs, ostype.GenericLinux)

	c.Assert(metadata, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cannot pack metadata for os GenericLinux on the gce provider")
}

var getDisksTests = []struct {
	osname string
	error  error
}{
	{"ubuntu", nil},
	{"suse", errors.New("os Suse is not supported on the gce provider")},
}

func (s *environBrokerSuite) TestGetDisks(c *tc.C) {
	for _, test := range getDisksTests {
		os := ostype.OSTypeForName(test.osname)
		diskSpecs, err := gce.GetDisks(c.Context(), "image-url", s.StartInstArgs.Constraints, os)
		if test.error != nil {
			c.Assert(err, tc.Equals, err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(diskSpecs, tc.HasLen, 1)
			diskSpec := diskSpecs[0]
			c.Assert(diskSpec.InitializeParams, tc.NotNil)
			c.Check(diskSpec.InitializeParams.GetDiskSizeGb(), tc.Equals, int64(10))
			c.Check(diskSpec.InitializeParams.GetSourceImage(), tc.Equals, "image-url")
		}
	}
}

func (s *environBrokerSuite) TestGetHardwareCharacteristics(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	hwc := gce.GetHardwareCharacteristics(env, s.spec, s.NewEnvironInstance(env, "inst-0"))

	c.Assert(hwc, tc.NotNil)
	c.Check(*hwc.Arch, tc.Equals, "amd64")
	c.Check(*hwc.AvailabilityZone, tc.Equals, "home-zone")
	c.Check(*hwc.CpuCores, tc.Equals, uint64(2))
	c.Check(*hwc.Mem, tc.Equals, uint64(3750))
	c.Check(*hwc.RootDisk, tc.Equals, uint64(15360))
}

func (s *environBrokerSuite) TestAllRunningInstances(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*computepb.Instance{s.NewComputeInstance("inst-0")}, nil)

	insts, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insts, tc.DeepEquals, []instances.Instance{s.NewEnvironInstance(env, "inst-0")})
}

func (s *environBrokerSuite) TestStopInstances(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0").Return(nil)

	err := env.StopInstances(c.Context(), "inst-0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStopInstancesInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0").Return(gce.InvalidCredentialError)

	err := env.StopInstances(c.Context(), "inst-0")
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}
