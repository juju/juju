// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/storage"
)

type environAZSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environAZSuite{})

func (s *environAZSuite) TestAvailabilityZonesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	_, err := env.AvailabilityZones(s.CallCtx)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZones(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "a-zone",
		Status: "UP",
	}, {
		Name:   "b-zone",
		Status: "DOWN",
	}, {
		Name:       "c-zone",
		Deprecated: &compute.DeprecationStatus{},
	}}, nil)

	zones, err := env.AvailabilityZones(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, gc.HasLen, 2)
	c.Check(zones[0].Name(), gc.Equals, "a-zone")
	c.Check(zones[0].Available(), jc.IsTrue)
	c.Check(zones[1].Name(), gc.Equals, "b-zone")
	c.Check(zones[1].Available(), jc.IsFalse)
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*compute.Instance{{
			Name: "inst-0",
			Zone: "home-zone",
		}, {
			Name: "inst-1",
			Zone: "home-a-zone",
		}}, nil)

	id := instance.Id("inst-0")
	ids := []instance.Id{id}
	zones, err := env.InstanceAvailabilityZoneNames(s.CallCtx, ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, map[instance.Id]string{
		id: "home-zone",
	})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	s.StartInstArgs.Placement = "zone=test-available"
	_, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environAZSuite) TestDeriveAvailabilityZones(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "test-available",
		Status: "UP",
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-available"
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"test-available"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"away-zone"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnavailable(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "test-unavailable",
		Status: "DOWN",
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-unavailable"
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Check(err, gc.ErrorMatches, `.*availability zone "test-unavailable" is DOWN`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnknown(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-unknown"
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown" not found`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesConflictsVolume(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}, {
		Name:   "away-zone",
		Status: "UP",
	}}, nil)

	s.StartInstArgs.Placement = "zone=home-zone"
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=home-zone": cannot create instance in zone "home-zone", as this will prevent attaching the requested disks in zone "away-zone"`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachments(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachmentsDifferentZones(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}, {
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	_, err := env.DeriveAvailabilityZones(s.CallCtx, s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, `cannot attach volumes from multiple availability zones: home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in home-zone, away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in away-zone`)
}
