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

var v365 = version.MustParse("3.6.5")

type steps365Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps365Suite{})

func (s *steps365Suite) TestSplitMigrationStatusMessages(c *gc.C) {
	step := findStateStep(c, v365, "split migration status messages")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
