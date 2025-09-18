// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/storage"
)

type environAZSuite struct {
	gce.BaseSuite
}

func TestEnvironAZSuite(t *testing.T) {
	tc.Run(t, &environAZSuite{})
}

func (s *environAZSuite) TestAvailabilityZonesInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	_, err := env.AvailabilityZones(c.Context())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZones(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("a-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("b-zone"),
		Status: ptr("DOWN"),
	}, {
		Name:       ptr("c-zone"),
		Deprecated: &computepb.DeprecationStatus{},
	}}, nil)

	zones, err := env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(zones, tc.HasLen, 2)
	c.Check(zones[0].Name(), tc.Equals, "a-zone")
	c.Check(zones[0].Available(), tc.IsTrue)
	c.Check(zones[1].Name(), tc.Equals, "b-zone")
	c.Check(zones[1].Available(), tc.IsFalse)
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNames(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return([]*computepb.Instance{{
			Name: ptr("inst-0"),
			Zone: ptr("home-zone"),
		}, {
			Name: ptr("inst-1"),
			Zone: ptr("home-a-zone"),
		}}, nil)

	id := instance.Id("inst-0")
	ids := []instance.Id{id}
	zones, err := env.InstanceAvailabilityZoneNames(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(zones, tc.DeepEquals, map[instance.Id]string{
		id: "home-zone",
	})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	s.StartInstArgs.Placement = "zone=test-available"
	_, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environAZSuite) TestDeriveAvailabilityZones(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("test-available"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("test-unavailable"),
		Status: ptr("DOWN"),
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-available"
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"test-available"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("away-zone"),
		Status: ptr("UP"),
	}}, nil)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"away-zone"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnavailable(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("test-unavailable"),
		Status: ptr("DOWN"),
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-unavailable"
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Check(err, tc.ErrorMatches, `.*availability zone "test-unavailable" is DOWN`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnknown(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)

	s.StartInstArgs.Placement = "zone=test-unknown"
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `invalid availability zone "test-unknown" not found`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesConflictsVolume(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.StartInstArgs.Placement = "zone=home-zone"
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `cannot create instance in zone "home-zone", as this will prevent attaching the requested disks in zone "away-zone"`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachments(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("away-zone"),
		Status: ptr("UP"),
	}}, nil)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachmentsDifferentZones(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}, {
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	_, err := env.DeriveAvailabilityZones(c.Context(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `cannot attach volumes from multiple availability zones: home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in home-zone, away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in away-zone`)
}
