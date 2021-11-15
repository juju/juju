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

var v2919 = version.MustParse("2.9.19")

type steps2919Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2919Suite{})

func (s *steps2919Suite) TestMigrateLegacyCrossModelTokens(c *gc.C) {
	step := findStateStep(c, v2919, `migrate legacy cross model tokens`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
