// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v272 = version.MustParse("2.7.2")

type steps272Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps272Suite{})

func (s *steps272Suite) TestCorrectServiceLogFilePath(c *gc.C) {
	step := findStep(c, v272, "ensure systemd files are located under /etc/systemd/system")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.AllMachines})
}
