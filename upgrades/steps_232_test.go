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

var v232 = version.MustParse("2.3.2")

type steps232Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps232Suite{})

func (s *steps232Suite) TestMoveOldAuditLog(c *gc.C) {
	step := findStateStep(c, v232, "move or drop the old audit log collection")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
