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

var v299 = version.MustParse("2.9.9")

type steps299Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps299Suite{})

func (s *steps299Suite) TestAddSpawnedTaskCountToOperations(c *gc.C) {
	step := findStateStep(c, v299, `add spawned task count to operations`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
