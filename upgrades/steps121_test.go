// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps121Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps121Suite{})

func (s *steps121Suite) TestStepsFor121a1(c *gc.C) {
	var expectedSteps = []string{
		"rename the user LastConnection field to LastLogin",
		"add environment uuid to state server doc",
		"add all users in state as environment users",
		"set environment owner and server uuid",
	}
	assertSteps(c, version.MustParse("1.21-alpha1"), expectedSteps)
}

func (s *steps121Suite) TestStepsFor121a2(c *gc.C) {
	var expectedSteps = []string{
		"prepend the environment UUID to the ID of all service docs",
		"prepend the environment UUID to the ID of all unit docs",
		"migrate charm archives into environment storage",
		"migrate custom image metadata into environment storage",
		"migrate tools into environment storage",
		"migrate individual unit ports to openedPorts collection",
		"create entries in meter status collection for existing units",
	}
	assertSteps(c, version.MustParse("1.21-alpha2"), expectedSteps)
}

func (s *steps121Suite) TestStepsFor121a3(c *gc.C) {
	var expectedSteps = []string{
		// It is important to keep the order of the following three steps:
		// 1.migrate machine instanceId, 2. Add env ID to  machine docs, 3.
		// Add env ID to instanceData docs. If the order changes, bad things
		// will happen.
		"migrate machine instanceId into instanceData",
		"prepend the environment UUID to the ID of all machine docs",
		"prepend the environment UUID to the ID of all instanceData docs",
		"prepend the environment UUID to the ID of all containerRef docs",
		"prepend the environment UUID to the ID of all reboot docs",
	}
	assertSteps(c, version.MustParse("1.21-alpha3"), expectedSteps)
}
