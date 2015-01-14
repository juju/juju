// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environAZSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environAZSuite{})

func (*environAZSuite) TestAvailabilityZones(c *gc.C) {
}

func (*environAZSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
}

func (*environAZSuite) TestParseAvailabilityZones(c *gc.C) {
}
