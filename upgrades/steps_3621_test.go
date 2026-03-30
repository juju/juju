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

var v3621 = version.MustParse("3.6.21")

type steps3621Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3621Suite{})

func (s *steps3621Suite) TestConvertScalingToCurrentOperationEnumField(c *gc.C) {
	step := findStateStep(c, v3621, "convert scaling field to enum")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
