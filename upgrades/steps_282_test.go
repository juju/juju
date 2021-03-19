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

var v282 = version.MustParse("2.8.2")

type steps282Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps282Suite{})

func (s *steps282Suite) TestResetDefaultRelationLimitInCharmMetadata(c *gc.C) {
	step := findStateStep(c, v282, "reset default limit to 0 for existing charm metadata")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
