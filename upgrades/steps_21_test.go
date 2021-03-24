// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v210 = version.MustParse("2.1.0")

type steps21Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps21Suite{})

func (s *steps21Suite) TestAddMigrationAttempt(c *gc.C) {
	step := findStateStep(c, v210, "add attempt to migration docs")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps21Suite) TestAddLocalCharmSequences(c *gc.C) {
	step := findStateStep(c, v210, "add sequences to track used local charm revisions")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
