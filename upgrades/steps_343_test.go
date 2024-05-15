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

var v343 = version.MustParse("3.4.3")

type steps342Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps342Suite{})

func (s *steps342Suite) TestAssignArchToContainers(c *gc.C) {
	step := findStateStep(c, v343, "assign architectures to container's instance data")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
