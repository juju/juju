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

var v3614 = version.MustParse("3.6.14")

type steps3614Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps3614Suite{})

func (s *steps3614Suite) TestConvertScalingToCurrentOperationEnumField(c *gc.C) {
	step := findStateStep(c, v3614, "convert scaling field to enum")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
