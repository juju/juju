// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
)

var v3612 = version.MustParse("3.6.12")

type steps3612Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3612Suite{})

func (s *steps3612Suite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	step := findStateStep(c, v3612, "populate application storage unique ID")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
