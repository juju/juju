// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type steps121Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps121Suite{})

func (s *steps121Suite) TestUpgradeOperationsContent(c *gc.C) {
	var expectedSteps = []string{
		"rename the user LastConnection field to LastLogin",
		"add environment uuid to state server doc",
		"add all users in state as environment users",
	}

	upgradeSteps := upgrades.StepsFor121()
	c.Assert(upgradeSteps, gc.HasLen, len(expectedSteps))
	assertExpectedSteps(c, upgradeSteps, expectedSteps)
}
