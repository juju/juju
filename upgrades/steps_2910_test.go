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

var v2910 = version.MustParse("2.9.10")

type steps2910Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2910Suite{})

func (s *steps2910Suite) TestTransformEmptyManifestsToNil(c *gc.C) {
	step := findStateStep(c, v2910, `transform empty manifests to nil`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
