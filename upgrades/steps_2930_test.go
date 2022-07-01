// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/upgrades"
)

var v2930 = version.MustParse("2.9.30")

type steps2930Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2930Suite{})

func (s *steps2930Suite) TestRemoveLocalCharmOriginChannels(c *gc.C) {
	step := findStateStep(c, v2930, "remove channels from local charm origins")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
