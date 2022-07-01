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

var v2917 = version.MustParse("2.9.17")

type steps2917Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2917Suite{})

func (s *steps2917Suite) TestDropLegacyAssumesSectionsFromCharmMetadata(c *gc.C) {
	step := findStateStep(c, v2917, `drop assumes keys from charm collection`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
