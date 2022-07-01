// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/testing"
	"github.com/juju/juju/v3/upgrades"
)

var v2912 = version.MustParse("2.9.12")

type steps2912Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2912Suite{})

func (s *steps2912Suite) TestTransformEmptyManifestsToNil(c *gc.C) {
	step := findStateStep(c, v2912, `ensure correct charm-origin risk`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
