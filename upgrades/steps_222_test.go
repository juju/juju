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

var v222 = version.MustParse("2.2.2")

type steps222Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps223Suite{})

func (s *steps222Suite) TestAddModelEnvironVersionStep(c *gc.C) {
	step := findStateStep(c, v222, "add environ-version to model docs")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
