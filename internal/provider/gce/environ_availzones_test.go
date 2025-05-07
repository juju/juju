// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/storage"
)

type environAZSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&environAZSuite{})

func (s *environAZSuite) TestAvailabilityZonesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := s.Env.AvailabilityZones(context.Background())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZones(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
		google.NewZone("b-zone", google.StatusUp, "", ""),
	}

	zones, err := s.Env.AvailabilityZones(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, tc.HasLen, 2)
	c.Check(zones[0].Name(), tc.Equals, "a-zone")
	c.Check(zones[0].Available(), jc.IsTrue)
	c.Check(zones[1].Name(), tc.Equals, "b-zone")
	c.Check(zones[1].Available(), jc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZonesDeprecated(c *tc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "DEPRECATED", "b-zone")

	c.Check(zone.Deprecated(), jc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZonesAPI(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{}

	_, err := s.Env.AvailabilityZones(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, tc.Equals, "us-east1")
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNames(c *tc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	id := instance.Id("spam")
	ids := []instance.Id{id}
	zones, err := s.Env.InstanceAvailabilityZoneNames(context.Background(), ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, map[instance.Id]string{
		id: "home-zone",
	})
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNamesAPIs(c *tc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	ids := []instance.Id{instance.Id("spam")}
	_, err := s.Env.InstanceAvailabilityZoneNames(context.Background(), ids)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeEnviron.CheckCalls(c, []gce.FakeCall{{
		FuncName: "GetInstances", Args: gce.FakeCallArgs{"switch": s.Env},
	}})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesInvalidCredentialError(c *tc.C) {
	s.StartInstArgs.Placement = "zone=test-available"
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environAZSuite) TestDeriveAvailabilityZones(c *tc.C) {
	s.StartInstArgs.Placement = "zone=test-available"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("test-available", google.StatusUp, "", ""),
	}
	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"test-available"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("az1", google.StatusDown, "", ""),
		google.NewZone("az2", google.StatusUp, "", ""),
	}
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "az2--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"az2"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnavailable(c *tc.C) {
	s.StartInstArgs.Placement = "zone=test-unavailable"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("test-unavailable", google.StatusDown, "", ""),
	}
	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Check(err, tc.ErrorMatches, `.*availability zone "test-unavailable" is DOWN`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesUnknown(c *tc.C) {
	s.StartInstArgs.Placement = "zone=test-unknown"
	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `invalid availability zone "test-unknown" not found`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesConflictsVolume(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("az1", google.StatusUp, "", ""),
		google.NewZone("az2", google.StatusUp, "", ""),
	}
	s.StartInstArgs.Placement = "zone=az1"
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "az2--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}
	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=az1": cannot create instance in zone "az1", as this will prevent attaching the requested disks in zone "az2"`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachments(c *tc.C) {
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}

	zones, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestDeriveAvailabilityZonesVolumeAttachmentsDifferentZones(c *tc.C) {
	s.StartInstArgs.VolumeAttachments = []storage.VolumeAttachmentParams{{
		VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}, {
		VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
	}}

	_, err := s.Env.DeriveAvailabilityZones(context.Background(), s.StartInstArgs)
	c.Assert(err, tc.ErrorMatches, `cannot attach volumes from multiple availability zones: home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in home-zone, away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4 is in away-zone`)
}
