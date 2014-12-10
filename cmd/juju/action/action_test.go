// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
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

	expected := "(?s).*^usage: juju action <command> .+"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + s.command.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + s.command.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	// Check that we've properly registered all subcommands
	s.checkHelpSubCommands(c)
}

func (s *ActionCommandSuite) checkHelpSubCommands(c *gc.C) {
	var expectedSubCommmands = [][]string{
		[]string{"defined", "WIP: show actions defined for a service"},
		[]string{"do", "WIP: queue an action for execution"},
		[]string{"fetch", "WIP: show results of an action by UUID"},
		[]string{"help", "show help on a command or other topic"},
		[]string{"kill", "TODO: remove an action from the queue"},
		[]string{"log", "TODO: fetch logged action results"},
		[]string{"queue", "TODO: show queued actions"},
		[]string{"status", "TODO: show status of action by id"},
		[]string{"wait", "TODO: wait for results of an action"},
	}

	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, gc.IsNil)

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
