// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/utils"
)

type instanceSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestAvailabilityZone(c *gc.C) {
	env := fakeZonedEnv{Environ: s.Environ}
	env.instZones = []string{"a_zone"}
	s.PatchValue(utils.PatchedGetEnvironment, func(st *state.State) (environs.Environ, error) {
		return &env, nil
	})

	zone, err := utils.AvailabilityZone(s.State, "id-1")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zone, gc.Equals, "a_zone")
}

func (s *instanceSuite) TestAvailabilityZoneUnsupported(c *gc.C) {
	// Trigger a not supported error.
	s.AssertConfigParameterUpdated(c, "broken", "InstanceAvailabilityZoneNames")

	_, err := utils.AvailabilityZone(s.State, "id-1")
	c.Check(err, jc.Satisfies, errors.IsNotSupported)
}
