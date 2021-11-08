// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v2813 = version.MustParse("2.8.13")

type steps2813Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2813Suite{})

func (s *steps2813Suite) TestAddSpawnedTaskCountToOperations(c *gc.C) {
	step := findStateStep(c, v2813, `add spawned task count to operations`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
