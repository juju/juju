// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
)

type UserCommandSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UserCommandSuite{})

var expectedUserCommmandNames = []string{}

func (s *UserCommandSuite) TestHelp(c *gc.C) {
	// Check the help output and check that we have correctly
	// registered all the sub commands by inspecting the help output.
	ctx, err := coretesting.RunCommand(c, NewUserCommand(), []string{"--help"})
	c.Assert(err, gc.IsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Matches,
		"(?s)usage: user <command> .+"+
			userCommandPurpose+".+"+
			userCommandDoc+".+")
}
