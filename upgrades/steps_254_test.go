// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/upgrades"
)

var v254 = version.MustParse("2.5.4")

type steps254Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps254Suite{})

func (s *steps254Suite) TestEnsureDefaultModificationStatus(c *gc.C) {
	step := findStateStep(c, v254, `ensure default modification status is set for machines`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps254Suite) TestEnsureApplicationDeviceConstraints(c *gc.C) {
	step := findStateStep(c, v254, `ensure device constraints exists for applications`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
