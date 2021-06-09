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

var v295 = version.MustParse("2.9.5")

type steps295Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps295Suite{})

func (s *steps295Suite) TestAddOriginToIPAddresses(c *gc.C) {
	step := findStateStep(c, v295, `change "dynamic" link-layer address configs to "dhcp"`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
