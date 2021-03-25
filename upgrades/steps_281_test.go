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

var v281 = version.MustParse("2.8.1")

type steps281Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps281Suite{})

func (s *steps281Suite) TestAddOriginToIPAddresses(c *gc.C) {
	step := findStateStep(c, v281, "add origin to IP addresses")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps281Suite) TestRemoveUnsupportedLinkLayer(c *gc.C) {
	step := findStateStep(c, v281, `remove "unsupported" link-layer device data`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps281Suite) AddBakeryConfig(c *gc.C) {
	step := findStateStep(c, v281, "add bakery config")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
