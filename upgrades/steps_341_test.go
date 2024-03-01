// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v341 = version.MustParse("3.4.1")

type steps341Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps341Suite{})

func (s *steps341Suite) TestFillInEmptyCharmhubTracks(c *gc.C) {
	step := findStateStep(c, v341, "fill in empty charmhub charm origin tracks to latest")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
