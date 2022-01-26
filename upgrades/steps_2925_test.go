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

var v2925 = version.MustParse("2.9.25")

type steps2925Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2925Suite{})

func (s *steps2925Suite) TestUpdateExternalControllerInfo(c *gc.C) {
	step := findStateStep(c, v2925, "remove invalid charm placeholders")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
