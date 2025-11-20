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

var v3613 = version.MustParse("3.6.13")

type steps3613Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3613Suite{})

func (s *steps3613Suite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	step := findStateStep(c, v3613, "populate application storage unique ID")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
