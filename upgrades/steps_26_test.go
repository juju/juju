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

var v26 = version.MustParse("2.6.0")

type steps26Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps26Suite{})

func (s *steps26Suite) TestUpdateInheritedControllerConfig(c *gc.C) {
	step := findStateStep(c, v26, `update k8s storage config`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps26Suite) TestRemoveInstanceCharmProfileDataCollection(c *gc.C) {
	step := findStateStep(c, v26, `remove instanceCharmProfileData collection`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
