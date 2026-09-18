// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v3629 = version.MustParse("3.6.29")

type steps3629Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3629Suite{})

func (s *steps3629Suite) TestFixApplicationCounts(c *gc.C) {
	step := findStateStep(c, v3629, "repair remote application relation counts")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps3629Suite) TestRemoveOrphanedApplicationRelations(c *gc.C) {
	step := findStateStep(c, v3629, "remove relations with dangling application references")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps3629Suite) TestRemoveOrphanedRelationDocs(c *gc.C) {
	step := findStateStep(c, v3629, "remove orphaned relation docs")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps3629Suite) TestRemoveOrphanedUnitStateRelations(c *gc.C) {
	step := findStateStep(c, v3629, "remove orphaned unit state relations")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
