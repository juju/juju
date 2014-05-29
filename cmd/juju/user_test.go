// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
)

type UserCommandSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UserCommandSuite{})

var expectedUserCommmandNames = []string{
	"add",
	"help",
}

func (s *UserCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := coretesting.RunCommand(c, NewUserCommand(), "--help")
	c.Assert(err, gc.IsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Matches,
		"(?s)usage: user <command> .+"+
			userCommandPurpose+".+"+
			userCommandDoc+".+")

	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var namesFound []string
	commandHelp := strings.SplitAfter(coretesting.Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		namesFound = append(namesFound, strings.TrimSpace(strings.Split(line, " - ")[0]))
	}
	c.Assert(namesFound, gc.DeepEquals, expectedUserCommmandNames)
}
