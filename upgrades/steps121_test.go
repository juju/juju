// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type steps121a1Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps121a1Suite{})

func (s *steps121a1Suite) TestUpgradeOperationsContent(c *gc.C) {
	var expectedSteps = []string{
		"rename the user LastConnection field to LastLogin",
		"add environment uuid to state server doc",
		"add all users in state as environment users",
		"set environment owner and server uuid",
	}

	upgradeSteps := upgrades.StepsFor121a1()
	c.Assert(upgradeSteps, gc.HasLen, len(expectedSteps))
	assertExpectedSteps(c, upgradeSteps, expectedSteps)
}

type steps121a2Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps121a2Suite{})

func (s *steps121a2Suite) TestUpgradeOperationsContent(c *gc.C) {
	var expectedSteps = []string{
		"prepend the environment UUID to the ID of all service docs",
		"prepend the environment UUID to the ID of all unit docs",
		"migrate charm archives into environment storage",
		"migrate custom image metadata into environment storage",
		"migrate tools into environment storage",
		"migrate individual unit ports to openedPorts collection",
		"create entries in meter status collection for existing units",
		"migrate machine instanceId into instanceData",
	}

	upgradeSteps := upgrades.StepsFor121a2()
	c.Assert(upgradeSteps, gc.HasLen, len(expectedSteps))
	assertExpectedSteps(c, upgradeSteps, expectedSteps)
}
