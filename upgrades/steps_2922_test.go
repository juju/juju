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

var v2922 = version.MustParse("2.9.22")

type steps2922Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2922Suite{})

func (s *steps2922Suite) TestRemoveOrphanedLinkLayerDevices(c *gc.C) {
	step := findStateStep(c, v2922, "remove link-layer devices without machines")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps2922Suite) TestRemoveUnusedLinkLayerDeviceProviderIDs(c *gc.C) {
	step := findStateStep(c, v2922, "remove unused link-layer device provider IDs")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
