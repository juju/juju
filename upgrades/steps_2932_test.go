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

var v2932 = version.MustParse("2.9.32")

type steps2932Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2932Suite{})

func (s *steps2932Suite) TestFixCharmhubLastPolltime(c *gc.C) {
	step := findStateStep(c, v2932, "add last poll time to charmhub resources")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
