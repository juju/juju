// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	. "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

type errorsSuite struct {
}

func (*errorsSuite) TestZoneIndependentErrorConforms(c *C) {
	err := fmt.Errorf("fly screens on a submarine: %w", environs.ErrAvailabilityZoneIndependent)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), jc.IsTrue)

	err = fmt.Errorf("replace with solid doors: %w", err)
	err = environs.ZoneIndependentError(err)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), jc.IsTrue)

	err = fmt.Errorf("or stay on dry land: %w", err)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), jc.IsTrue)
}

func (*errorsSuite) TestZoneIndependentErrorIsLocationer(c *C) {
	err := environs.ZoneIndependentError(errors.New("lets not talk about submarine's any more"))
	_, ok := err.(errors.Locationer)
	c.Assert(ok, jc.IsTrue)
}
