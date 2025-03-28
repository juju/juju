// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
)

type zoneSuite struct {
	jujutesting.IsolationSuite

	zones AvailabilityZones
}

var _ = gc.Suite(&zoneSuite{})

func (s *zoneSuite) SetUpTest(c *gc.C) {
	s.zones = AvailabilityZones{
		&az{name: "zone1", available: true},
		&az{name: "zone2"},
	}

	s.IsolationSuite.SetUpTest(c)
}

func (s *zoneSuite) TestAvailabilityZones(c *gc.C) {
	c.Assert(s.zones.Validate("zone1"), jc.ErrorIsNil)
	c.Assert(s.zones.Validate("zone2"), gc.ErrorMatches, `zone "zone2" is unavailable`)
	c.Assert(s.zones.Validate("zone3"), jc.ErrorIs, coreerrors.NotValid)
}

type az struct {
	name      string
	available bool
}

var _ = AvailabilityZone(&az{})

func (a *az) Name() string {
	return a.name
}

func (a *az) Available() bool {
	return a.available
}
