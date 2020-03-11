// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v280 = version.MustParse("2.8.0")

type steps28Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps28Suite{})

func (s *steps28Suite) TestIncrementTasksSequence(c *gc.C) {
	step := findStateStep(c, v280, "increment tasks sequence by 1")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps28Suite) TestPopulateRebootHandledFlagsForDeployedUnits(c *gc.C) {
	step := findStep(c, v280, "ensure currently running units do not fire start hooks thinking a reboot has occurred")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
}
