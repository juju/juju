// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v331 = version.MustParse("3.3.1")

type steps331Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps331Suite{})

func (s *steps331Suite) TestConvertApplicationOfferTokenKeys(c *gc.C) {
	step := findStateStep(c, v331, "convert application offer token keys")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
