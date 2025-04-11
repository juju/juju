// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v366 = version.MustParse("3.6.6")

type steps366Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps366Suite{})

func (s *steps366Suite) TestUpgradeAddJumpHostKey(c *gc.C) {
	step := findStateStep(c, v366, "add ssh jump host key")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
