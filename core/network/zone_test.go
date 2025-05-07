// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	coreerrors "github.com/juju/juju/core/errors"
)

type zoneSuite struct {
	jujutesting.IsolationSuite

	zones AvailabilityZones
}

var _ = tc.Suite(&zoneSuite{})

func (s *zoneSuite) SetUpTest(c *tc.C) {
	s.zones = AvailabilityZones{
		&az{name: "zone1", available: true},
		&az{name: "zone2"},
	}

	s.IsolationSuite.SetUpTest(c)
}

func (s *zoneSuite) TestAvailabilityZones(c *tc.C) {
	c.Assert(s.zones.Validate("zone1"), tc.ErrorIsNil)
	c.Assert(s.zones.Validate("zone2"), tc.ErrorMatches, `zone "zone2" is unavailable`)
	c.Assert(s.zones.Validate("zone3"), tc.ErrorIs, coreerrors.NotValid)
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
