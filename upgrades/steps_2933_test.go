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

var v2933 = version.MustParse("2.9.33")

type steps2933Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2933Suite{})

func (s *steps2933Suite) TestRemoveUseFloatingIPConfigFalse(c *gc.C) {
	step := findStateStep(c, v2933, "remove use-floating-ip=false from model config")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
