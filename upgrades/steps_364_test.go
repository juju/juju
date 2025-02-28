// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v364 = version.MustParse("3.6.4")

type steps364Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps364Suite{})

func (s *steps364Suite) TestAddsVirtualHostKeys(c *gc.C) {
	step := findStateStep(c, v364, "add virtual host keys")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
