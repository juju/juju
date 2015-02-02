// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var expectedSubCommmandNames = []string{
	"help",
	"show",
}

type storageSuite struct {
	BaseStorageSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) checkHelpCommands(c *gc.C) {
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

func (s *storageSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, jc.ErrorIsNil)

	expected := "(?s)usage: juju storage <command> .+"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + s.command.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + s.command.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)

	s.checkHelpCommands(c)
}
