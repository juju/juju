// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/testing"
)

var expectedSubCommmandNames = []string{
	"create",
	"help",
}

type BaseSpaceSuite struct {
	jujutesting.IsolationSuite
	command *cmd.SuperCommand
}

func (s *BaseSpaceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.command = space.NewSuperCommand().(*cmd.SuperCommand)
}

type spaceSuite struct {
	BaseSpaceSuite
}

func (s *spaceSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
}

var _ = gc.Suite(&spaceSuite{})

// runCommand runs the api-endpoints command with the given arguments
// and returns the output and any error.
func (s *BaseSpaceSuite) runCommand(c *gc.C, args ...string) (string, string, error) {
	ctx, err := testing.RunCommand(c, s.command, args...)
	if err != nil {
		return "", "", err
	}
	return testing.Stdout(ctx), testing.Stderr(ctx), nil
}

func (s *spaceSuite) checkHelpCommands(c *gc.C) {
	stdout, _, err := s.runCommand(c, "--help")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var namesFound []string
	commandHelp := strings.SplitAfter(stdout, "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		name := strings.TrimSpace(strings.Split(line, " - ")[0])
		namesFound = append(namesFound, name)
	}

	c.Check(namesFound, gc.DeepEquals, expectedSubCommmandNames)
}

func (s *spaceSuite) TestHelp(c *gc.C) {
	stdout, _, err := s.runCommand(c, "--help")
	c.Assert(err, jc.ErrorIsNil)

	expected := "(?s)usage: juju space <command> .+"
	c.Check(stdout, gc.Matches, expected)
	expected = "(?sm).*^purpose: " + s.command.Purpose + "$.*"
	c.Check(stdout, gc.Matches, expected)
	expected = "(?sm).*^" + s.command.Doc + "$.*"
	c.Check(stdout, gc.Matches, expected)

	s.checkHelpCommands(c)
}

// testSubcmdHelp tests that a subcommand is wired up and returning the expected help text.
func (s *BaseSpaceSuite) testSubcmdHelp(c *gc.C, info *cmd.Info, subcmd string) {
	stdout, _, err := s.runCommand(c, subcmd, "--help")
	c.Assert(err, jc.ErrorIsNil)

	expected := fmt.Sprintf("(?s)\\Qusage: juju space %s [options] %s\\E.*", info.Name, info.Args)
	c.Check(stdout, gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(stdout, gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(stdout, gc.Matches, expected)
}
