// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v3622 = version.MustParse("3.6.22")

type steps3622Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3622Suite{})

func (s *steps3622Suite) TestExposeControllerApplication(c *gc.C) {
	step := findStateStep(c, v3622, "expose controller application")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps3622Suite) TestConvertScalingToCurrentOperationEnumField(c *gc.C) {
	step := findStateStep(c, v3622, "convert scaling field to enum")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
