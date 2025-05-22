// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

type errorsSuite struct {
}

func TestErrorsSuite(t *testing.T) {
	tc.Run(t, &errorsSuite{})
}

func (*errorsSuite) TestZoneIndependentErrorConforms(c *tc.C) {
	err := fmt.Errorf("fly screens on a submarine: %w", environs.ErrAvailabilityZoneIndependent)
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)

	err = fmt.Errorf("replace with solid doors: %w", err)
	err = environs.ZoneIndependentError(err)
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)

	err = fmt.Errorf("or stay on dry land: %w", err)
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}
