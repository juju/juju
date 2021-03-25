// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v265 = version.MustParse("2.6.5")

type steps265Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps265Suite{})

func (s *steps265Suite) TestAddModelLogsSize(c *gc.C) {
	step := findStateStep(c, v265, "add models-logs-size to controller config")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
