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

var v2924 = version.MustParse("2.9.24")

type steps2924Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2924Suite{})

func (s *steps2924Suite) TestUpdateExternalControllerInfo(c *gc.C) {
	step := findStateStep(c, v2924, "update remote application external controller info")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps2924Suite) TestRemoveInvalidCharmPlaceholders(c *gc.C) {
	step := findStateStep(c, v2924, "remove invalid charm placeholders")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps2924Suite) TestUpdateCharmOriginAfterSetSeries(c *gc.C) {
	step := findStateStep(c, v2924, "update charm origin to facilitate charm refresh after set-series")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
