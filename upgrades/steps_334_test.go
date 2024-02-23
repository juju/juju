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

var v334 = version.MustParse("3.3.4")

type steps334Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps334Suite{})

func (s *steps334Suite) TestFillInEmptyCharmhubTracks(c *gc.C) {
	step := findStateStep(c, v334, "fill in empty charmhub charm origin tracks to latest")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
