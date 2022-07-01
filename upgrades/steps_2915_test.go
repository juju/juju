// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/upgrades"
)

var v2915 = version.MustParse("2.9.15")

type steps2915Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2915Suite{})

func (s *steps2915Suite) TestRemoveOrphanedCrossModelProxies(c *gc.C) {
	step := findStateStep(c, v2915, `remove orphaned cross model proxies`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
