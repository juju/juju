// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v234 = version.MustParse("2.3.4")

type steps234Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps234Suite{})

func (s *steps234Suite) TestDeleteCloudImageMetadata(c *gc.C) {
	step := findStateStep(c, v234, "delete cloud image metadata cache")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
