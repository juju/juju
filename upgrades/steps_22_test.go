// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v220 = version.MustParse("2.2.0")

type steps22Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps22Suite{})

func (s *steps22Suite) TestAddNonDetachableStorageMachineId(c *gc.C) {
	step := findStateStep(c, v220, "add machineid to non-detachable storage docs")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
