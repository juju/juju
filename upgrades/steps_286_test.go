// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v286 = version.MustParse("2.8.6")

type steps286Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps286Suite{})

func (s *steps286Suite) TestRemoveUnusedLinkLayerDeviceProviderIDs(c *gc.C) {
	step := findStateStep(c, v286, "remove unused link-layer device provider IDs")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
