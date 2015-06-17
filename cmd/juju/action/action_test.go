// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type ActionCommandSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ActionCommandSuite{})

func (s *ActionCommandSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *ActionCommandSuite) TestHelp(c *gc.C) {
	// Check the normal help for any command
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, gc.IsNil)

	expected := "(?s).*^usage: juju action \\[options\\] <command> .+"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	supercommand, ok := s.command.(*cmd.SuperCommand)
	c.Check(ok, jc.IsTrue)
	expected = "(?sm).*^purpose: " + supercommand.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + supercommand.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	// Check that we've properly registered all subcommands
	s.checkHelpSubCommands(c, ctx)
}

func (s *ActionCommandSuite) checkHelpSubCommands(c *gc.C, ctx *cmd.Context) {
	var expectedSubCommmands = [][]string{
		{"defined", "show actions defined for a service"},
		{"do", "queue an action for execution"},
		{"fetch", "show results of an action by ID"},
		{"help", "show help on a command or other topic"},
		{"status", "show results of all actions filtered by optional ID prefix"},
	}

	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var subsFound [][]string
	commandHelp := strings.SplitAfter(testing.Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		subcommand := strings.Split(line, " - ")
		c.Assert(len(subcommand), gc.Equals, 2)
		name := strings.TrimSpace(subcommand[0])
		desc := strings.TrimSpace(subcommand[1])
		subsFound = append(subsFound, []string{name, desc})
	}

	c.Check(subsFound, jc.DeepEquals, expectedSubCommmands)
}
