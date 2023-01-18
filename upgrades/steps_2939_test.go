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

var v2939 = version.MustParse("2.9.39")

type steps2939Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2939Suite{})

func (s *steps2939Suite) TestCorrectControllerConfigDurations(c *gc.C) {
	step := findStateStep(c, v2939, "correct stored durations in controller config")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
