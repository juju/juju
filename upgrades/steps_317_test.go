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

var v317 = version.MustParse("3.1.7")

type steps317Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps317Suite{})

func (s *steps317Suite) TestEnsureApplicationCharmOriginsHaveRevisions(c *gc.C) {
	step := findStateStep(c, v317, "ensure application charm origins have revisions")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
