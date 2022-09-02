// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v2934 = version.MustParse("2.9.34")

type steps2934Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2934Suite{})

func (s *steps2934Suite) TestCharmOriginChannelMustHaveTrack(c *gc.C) {
	step := findStateStep(c, v2934, "add latest as charm-origin channel track if not specified")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
