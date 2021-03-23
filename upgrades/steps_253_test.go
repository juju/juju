// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v253 = version.MustParse("2.5.3")

type steps253Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps253Suite{})

func (s *steps253Suite) TestUpdateInheritedControllerConfig(c *gc.C) {
	step := findStateStep(c, v253, `update inherited controller config global key`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
