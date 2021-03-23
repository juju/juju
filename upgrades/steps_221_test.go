// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v221 = version.MustParse("2.2.1")

type steps221Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps221Suite{})

func (s *steps221Suite) TestUpdateStatusHistoryHookSettings(c *gc.C) {
	step := findStateStep(c, v221, "add update-status hook config settings")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps221Suite) TestCorrectRelationUnitCounts(c *gc.C) {
	step := findStateStep(c, v221, "correct relation unit counts for subordinates")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
