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

func (s *steps121Suite) TestStateStepsFor121(c *gc.C) {
	expected := []string{
		// Environment UUID related migrations should come first as
		// other upgrade steps may rely on them.
		"add environment uuid to state server doc",
		"set environment owner and server uuid",
		// It is important to keep the order of the following three steps:
		// 1.migrate machine instanceId, 2. Add env ID to  machine docs, 3.
		// Add env ID to instanceData docs. If the order changes, bad things
		// will happen.
		"migrate machine instanceId into instanceData",
		"prepend the environment UUID to the ID of all machine docs",
		"prepend the environment UUID to the ID of all instanceData docs",
		"prepend the environment UUID to the ID of all containerRef docs",
		"prepend the environment UUID to the ID of all service docs",
		"prepend the environment UUID to the ID of all unit docs",
		"prepend the environment UUID to the ID of all reboot docs",
		"prepend the environment UUID to the ID of all relations docs",
		"prepend the environment UUID to the ID of all relationscopes docs",
		"prepend the environment UUID to the ID of all minUnit docs",
		"prepend the environment UUID to the ID of all cleanup docs",
		"prepend the environment UUID to the ID of all sequence docs",

		// Non-environment UUID upgrade steps follow.
		"rename the user LastConnection field to LastLogin",
		"add all users in state as environment users",
		"migrate custom image metadata into environment storage",
		"migrate tools into environment storage",
		"migrate individual unit ports to openedPorts collection",
		"create entries in meter status collection for existing units",
		"migrate machine jobs into ones with JobManageNetworking based on rules",
	}
	assertStateSteps(c, version.MustParse("1.21.0"), expected)
}

func (s *steps121Suite) TestStepsFor121(c *gc.C) {
	assertSteps(c, version.MustParse("1.21.0"), []string{})
}
