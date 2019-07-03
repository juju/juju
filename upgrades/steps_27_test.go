// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v27 = version.MustParse("2.7.0")

type steps27Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps27Suite{})

func (s *steps27Suite) TestCreateControllerNodes(c *gc.C) {
	step := findStateStep(c, v27, `add controller node docs`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps27Suite) TestAddSpaceIdToSpaceDocs(c *gc.C) {
	step := findStateStep(c, v27, `recreated spaces with IDs`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
