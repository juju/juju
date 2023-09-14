// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

type errorsSuite struct {
}

var _ = gc.Suite(&errorsSuite{})

func (*errorsSuite) TestZoneIndependentErrorConforms(c *gc.C) {
	err := fmt.Errorf("fly screens on a submarine: %w", environs.ErrAvailabilityZoneIndependent)
	c.Assert(err, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)

	err = fmt.Errorf("replace with solid doors: %w", err)
	err = environs.ZoneIndependentError(err)
	c.Assert(err, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)

	err = fmt.Errorf("or stay on dry land: %w", err)
	c.Assert(err, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}
