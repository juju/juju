// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type HelpToolSuite struct {
	home *testing.FakeHome
}

var _ = Suite(&HelpToolSuite{})

func (suite *HelpToolSuite) SetUpTest(c *C) {
	suite.home = testing.MakeSampleHome(c)
}

func (suite *HelpToolSuite) TearDownTest(c *C) {
	suite.home.Restore()
}

func (suite *HelpToolSuite) TestHelpToolHelp(c *C) {
	output := badrun(c, 0, "help", "help-tool")
	c.Assert(output, Equals, `usage: juju help-tool [tool]
purpose: show help on a juju charm tool
`)
}

func (suite *HelpToolSuite) TestHelpTool(c *C) {
	expectedNames := []string{
		"close-port",
		"config-get",
		"juju-log",
		"open-port",
		"relation-get",
		"relation-ids",
		"relation-list",
		"relation-set",
		"unit-get",
	}
	output := badrun(c, 0, "help-tool")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		lines[i] = strings.Fields(line)[0]
	}
	c.Assert(lines, DeepEquals, expectedNames)
}

func (suite *HelpToolSuite) TestHelpToolName(c *C) {
	output := badrun(c, 0, "help-tool", "relation-get")
	expectedHelp := `usage: relation-get \[options\] <key> <unit id>
purpose: get relation settings

options:
(.|\n)*
relation-get prints the value(.|\n)*`
	c.Assert(output, Matches, expectedHelp)
}
