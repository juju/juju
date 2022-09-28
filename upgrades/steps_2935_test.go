// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v2935 = version.MustParse("2.9.35")

type steps2935Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2935Suite{})

func (s *steps2935Suite) TestRemoveDefaultSeriesFromModelConfig(c *gc.C) {
	step := findStateStep(c, v2935, "remove default-series value from model-config")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
