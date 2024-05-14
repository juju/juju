// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v335 = version.MustParse("3.3.5")

type steps335Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps335Suite{})

func (s *steps335Suite) TestAssignArchToContainers(c *gc.C) {
	step := findStateStep(c, v335, "assign architectures to container's instance data")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
