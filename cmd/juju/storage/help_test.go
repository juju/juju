// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type HelpStorageSuite struct {
	testing.FakeJujuHomeSuite

	command cmd.Command
}

func (s *HelpStorageSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
}

func (s *HelpStorageSuite) TearDownTest(c *gc.C) {
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *HelpStorageSuite) assertHelp(c *gc.C, expectedNames []string) {
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, jc.ErrorIsNil)

	super, ok := s.command.(*cmd.SuperCommand)
	c.Assert(ok, jc.IsTrue)
	expected := "(?sm).*^purpose: " + super.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + super.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	s.checkHelpCommands(c, ctx, expectedNames)
}

func (s *HelpStorageSuite) checkHelpCommands(c *gc.C, ctx *cmd.Context, expectedNames []string) {
	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var namesFound []string
	commandHelp := strings.SplitAfter(testing.Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		name := strings.TrimSpace(strings.Split(line, " - ")[0])
		namesFound = append(namesFound, name)
	}
	c.Check(namesFound, gc.DeepEquals, expectedNames)
}
