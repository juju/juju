// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type zoneSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&zoneSuite{})

func (s *zoneSuite) TestAvailabilityZoneName(c *gc.C) {
	//zone := google.NewZone("spam", "UP")
	//name := zone.Name()

	//c.Check(name, gc.Equals, "spam")
}

func (s *zoneSuite) TestAvailabilityZoneStatus(c *gc.C) {
}

func (s *zoneSuite) TestAvailabilityZoneAvailable(c *gc.C) {
}

func (s *zoneSuite) TestZoneName(c *gc.C) {
}
