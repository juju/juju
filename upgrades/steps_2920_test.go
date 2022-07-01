// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/testing"
	"github.com/juju/juju/v3/upgrades"
)

var v2920 = version.MustParse("2.9.20")

type steps2920Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2920Suite{})

func (s *steps2920Suite) TestCleanupDeadAssignUnits(c *gc.C) {
	step := findStateStep(c, v2920, `clean up assignUnits for dead and removed applications`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
