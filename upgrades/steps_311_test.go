// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v311 = version.MustParse("3.1.1")

type steps311Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps311Suite{})

func (s *steps311Suite) TestRemoveOrphanedSecretRevisions(c *gc.C) {
	step := findStateStep(c, v311, "remove orphaned secret permissions")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
