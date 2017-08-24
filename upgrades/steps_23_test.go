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

var v23 = version.MustParse("2.3.0")

type steps23Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps23Suite{})

func (s *steps23Suite) TestAddModelType(c *gc.C) {
	step := findStateStep(c, v23, "add a 'type' field to model documents")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
