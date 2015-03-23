// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/testing"
)

var expectedSubCommmandNames = []string{
	"help",
}

type BaseSpaceSuite struct {
	command *cmd.SuperCommand
}

func (s *BaseSpaceSuite) SetUpTest(c *gc.C) {
	s.command = space.NewSuperCommand().(*cmd.SuperCommand)
}

type spaceSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) checkHelpCommands(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var namesFound []string
	commandHelp := strings.SplitAfter(testing.Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		name := strings.TrimSpace(strings.Split(line, " - ")[0])
		namesFound = append(namesFound, name)
	}
	c.Check(namesFound, gc.DeepEquals, expectedSubCommmandNames)
}

func (s *spaceSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, jc.ErrorIsNil)

	expected := "(?s)usage: juju space <command> .+"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + s.command.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + s.command.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	s.checkHelpCommands(c)
}
