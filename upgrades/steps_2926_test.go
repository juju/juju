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

var v2926 = version.MustParse("2.9.26")

type steps2926Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2926Suite{})

func (s *steps2926Suite) TestSetContainerAddressOriginToMachine(c *gc.C) {
	step := findStateStep(c, v2926, `set container address origins to "machine"`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps2926Suite) TestUpdateCharmOriginAfterSetSeries(c *gc.C) {
	step := findStateStep(c, v2926, "update charm origin to facilitate charm refresh after set-series")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
