// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type zoneSuite struct {
	google.BaseSuite

	raw  compute.Zone
	zone google.AvailabilityZone
}

var _ = gc.Suite(&zoneSuite{})

func (s *zoneSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.raw = compute.Zone{
		Name:   "c-zone",
		Status: google.StatusUp,
	}
	s.zone = google.NewAvailabilityZone(&s.raw)
}

func (s *zoneSuite) TestAvailabilityZoneName(c *gc.C) {
	c.Check(s.zone.Name(), gc.Equals, "c-zone")
}

func (s *zoneSuite) TestAvailabilityZoneStatus(c *gc.C) {
	c.Check(s.zone.Status(), gc.Equals, "UP")
}

func (s *zoneSuite) TestAvailabilityZoneAvailable(c *gc.C) {
	c.Check(s.zone.Available(), jc.IsTrue)
}

func (s *zoneSuite) TestAvailabilityZoneAvailableFalse(c *gc.C) {
	s.raw.Status = google.StatusDown
	c.Check(s.zone.Available(), jc.IsFalse)
}

func (s *zoneSuite) TestAvailabilityZoneNotDeprecated(c *gc.C) {
	c.Check(s.zone.Deprecated(), jc.IsFalse)
}

func (s *zoneSuite) TestAvailabilityZoneDeprecated(c *gc.C) {
	s.raw.Deprecated = &compute.DeprecationStatus{
		State: "DEPRECATED",
	}
	c.Check(s.zone.Deprecated(), jc.IsTrue)
}
