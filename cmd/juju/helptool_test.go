// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type HelpToolSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&HelpToolSuite{})

func (suite *HelpToolSuite) SetUpTest(c *gc.C) {
	suite.home = testing.MakeSampleHome(c)
}

func (suite *HelpToolSuite) TearDownTest(c *gc.C) {
	suite.home.Restore()
}

func (suite *HelpToolSuite) TestHelpToolHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-tool")
	c.Assert(output, gc.Equals, `usage: juju help-tool [tool]
purpose: show help on a juju charm tool
`)
}

func (suite *HelpToolSuite) TestHelpTool(c *gc.C) {
	expectedNames := jujuc.CommandNames()
	output := badrun(c, 0, "help-tool")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		lines[i] = strings.Fields(line)[0]
	}
	c.Assert(lines, gc.DeepEquals, expectedNames)
}

func (suite *HelpToolSuite) TestHelpToolName(c *gc.C) {
	output := badrun(c, 0, "help-tool", "relation-get")
	expectedHelp := `usage: relation-get \[options\] <key> <unit id>
purpose: get relation settings

options:
(.|\n)*
relation-get prints the value(.|\n)*`
	c.Assert(output, gc.Matches, expectedHelp)
}
