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

var v3615 = version.MustParse("3.6.15")

type steps3615Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3615Suite{})

func (s *steps3615Suite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	step := findStateStep(c, v3615, "open controller api port in state")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
