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

var v223 = version.MustParse("2.2.3")

type steps223Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps223Suite{})

func (s *steps223Suite) TestAddActionPruneSettings(c *gc.C) {
	step := findStateStep(c, v223, "add max-action-age and max-action-size config settings")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
