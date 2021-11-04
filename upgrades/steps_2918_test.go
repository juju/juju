// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v2918 = version.MustParse("2.9.18")

type steps2918Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2918Suite{})

func (s *steps2918Suite) TestRemoveUnusedLinkLayerDeviceProviderIDs(c *gc.C) {
	step := findStateStep(c, v2918, "remove unused link-layer device provider IDs")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
