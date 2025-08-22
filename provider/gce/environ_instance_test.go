// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/gce"
)

type environInstSuite struct {
	gce.BaseSuite

	zones []*compute.Zone
}

var _ = gc.Suite(&environInstSuite{})

func (s *environInstSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.zones = []*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}, {
		Name:   "away-zone",
		Status: "UP",
	}}
}

func (s *environInstSuite) TestInstancesNotFound(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	ids := []instance.Id{"spam", "eggs", "ham"}
	_, err := env.Instances(s.CallCtx, ids)
	c.Assert(err, jc.ErrorIs, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesEmptyArg(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	_, err := env.Instances(s.CallCtx, nil)
	c.Assert(err, jc.ErrorIs, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	failure := errors.New("<unknown>")
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(nil, failure)

	ids := []instance.Id{"inst-0"}
	_, err := env.Instances(s.CallCtx, ids)
	c.Assert(err, jc.ErrorIs, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	ids := []instance.Id{"inst-0", "inst-1"}
	insts, err := env.Instances(s.CallCtx, ids)
	c.Assert(insts, jc.DeepEquals, []instances.Instance{s.NewEnvironInstance(c, env, "inst-0"), nil})
	c.Assert(err, jc.ErrorIs, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	ids := []instance.Id{"inst-1"}
	insts, err := env.Instances(s.CallCtx, ids)

	c.Assert(insts, jc.DeepEquals, []instances.Instance{nil})
	c.Assert(err, jc.ErrorIs, environs.ErrNoInstances)
}

func (s *environInstSuite) TestBasicInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	ids := []instance.Id{"inst-0"}
	insts, err := env.Instances(s.CallCtx, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, jc.DeepEquals, []instances.Instance{s.NewEnvironInstance(c, env, "inst-0")})
}

func (s *environInstSuite) TestControllerInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	inst := s.NewComputeInstance(c, "inst-0")
	inst.Metadata = &compute.Metadata{Items: []*compute.MetadataItems{{
		Key:   "juju-is-controller",
		Value: ptr("true"),
	}, {
		Key:   "juju-controller-uuid",
		Value: ptr(s.ControllerUUID),
	}}}
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{inst, s.NewComputeInstance(c, "inst-1")}, nil)

	ids, err := env.ControllerInstances(s.CallCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, jc.DeepEquals, []instance.Id{"inst-0"})
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)

	_, err := env.ControllerInstances(s.CallCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIs, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestParsePlacement(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}}, nil)

	placement, err := gce.ParsePlacement(env, s.CallCtx, "zone=home-zone")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(placement, jc.DeepEquals, &compute.Zone{Name: "home-zone", Status: "UP"})
}

func (s *environInstSuite) TestParsePlacementZoneFailure(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	failure := errors.New("<unknown>")

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, failure)

	_, err := gce.ParsePlacement(env, s.CallCtx, "zone=home-zone")
	c.Assert(err, jc.ErrorIs, failure)
}

func (s *environInstSuite) TestParsePlacementMissingDirective(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	_, err := gce.ParsePlacement(env, s.CallCtx, "a-zone")

	c.Assert(err, gc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestParsePlacementUnknownDirective(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	_, err := gce.ParsePlacement(env, s.CallCtx, "inst=spam")

	c.Assert(err, gc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestInstanceInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	mem := uint64(1025)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	_, err := env.InstanceTypes(s.CallCtx, constraints.Value{Mem: &mem})
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environInstSuite) TestListMachineTypes(c *gc.C) {
	// If no zone is available, no machine types will be pulled.
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "DOWN",
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*compute.MachineType{{
		Id:           0,
		Name:         "n1-standard-1",
		GuestCpus:    int64(2),
		MemoryMb:     int64(4096),
		Architecture: "amd64",
	}}, nil)

	_, err := env.InstanceTypes(s.CallCtx, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "no instance types in  matching constraints.*")

	// If a non-empty list of zones is available , we will make an API call
	// to fetch the available machine types.
	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}}, nil)

	mem := uint64(1025)
	types, err := env.InstanceTypes(s.CallCtx, constraints.Value{Mem: &mem})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, jc.DeepEquals, []instances.InstanceType{{
		Id:         "0",
		Name:       "n1-standard-1",
		CpuCores:   uint64(2),
		Mem:        uint64(4096),
		Arch:       "amd64",
		VirtType:   ptr("kvm"),
		Networking: instances.InstanceTypeNetworking{},
	}})

}

func (s *environInstSuite) TestAdoptResources(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return([]*compute.Instance{s.NewComputeInstance(c, "inst-0")}, nil)
	s.MockService.EXPECT().UpdateMetadata(gomock.Any(), tags.JujuController, "other-uuid", "inst-0")

	err := env.AdoptResources(s.CallCtx, "other-uuid", version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environInstSuite) TestAdoptResourcesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return(nil, gce.InvalidCredentialError)

	err := env.AdoptResources(s.CallCtx, "other-uuid", version.MustParse("1.2.3"))
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}
