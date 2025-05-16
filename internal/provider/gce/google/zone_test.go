// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

type zoneSuite struct {
	google.BaseSuite

	raw  compute.Zone
	zone google.AvailabilityZone
}

func TestZoneSuite(t *stdtesting.T) { tc.Run(t, &zoneSuite{}) }
func (s *zoneSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.raw = compute.Zone{
		Name:   "c-zone",
		Status: google.StatusUp,
	}
	s.zone = google.NewAvailabilityZone(&s.raw)
}

func (s *zoneSuite) TestAvailabilityZoneName(c *tc.C) {
	c.Check(s.zone.Name(), tc.Equals, "c-zone")
}

func (s *zoneSuite) TestAvailabilityZoneStatus(c *tc.C) {
	c.Check(s.zone.Status(), tc.Equals, "UP")
}

func (s *zoneSuite) TestAvailabilityZoneAvailable(c *tc.C) {
	c.Check(s.zone.Available(), tc.IsTrue)
}

func (s *zoneSuite) TestAvailabilityZoneAvailableFalse(c *tc.C) {
	s.raw.Status = google.StatusDown
	c.Check(s.zone.Available(), tc.IsFalse)
}

func (s *zoneSuite) TestAvailabilityZoneNotDeprecated(c *tc.C) {
	c.Check(s.zone.Deprecated(), tc.IsFalse)
}

func (s *zoneSuite) TestAvailabilityZoneDeprecated(c *tc.C) {
	s.raw.Deprecated = &compute.DeprecationStatus{
		State: "DEPRECATED",
	}
	c.Check(s.zone.Deprecated(), tc.IsTrue)
}
