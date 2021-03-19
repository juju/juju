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

var v245 = version.MustParse("2.4.5")

type steps245Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps245Suite{})

func (s *steps245Suite) TestCorrectServiceLogFilePath(c *gc.C) {
	step := findStep(c, v245, "update exec.start.sh log path if incorrect")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.AllMachines})
}
