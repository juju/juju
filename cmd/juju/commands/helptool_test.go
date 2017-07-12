// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"runtime"
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type HelpToolSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&HelpToolSuite{})

func (suite *HelpToolSuite) TestHelpToolHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-tool")
	c.Assert(output, gc.Equals, `Usage: juju help-tool [tool]

Summary:
Show help on a Juju charm tool.
`)
}

var expectedCommands = []string{
	"action-fail",
	"action-get",
	"action-set",
	"add-metric",
	"application-version-set",
	"close-port",
	"config-get",
	"is-leader",
	"juju-log",
	"juju-reboot",
	"leader-get",
	"leader-set",
	"network-get",
	"open-port",
	"opened-ports",
	"payload-register",
	"payload-status-set",
	"payload-unregister",
	"relation-get",
	"relation-ids",
	"relation-list",
	"relation-set",
	"resource-get",
	"status-get",
	"status-set",
	"storage-add",
	"storage-get",
	"storage-list",
	"unit-get",
}

func (suite *HelpToolSuite) TestHelpTool(c *gc.C) {
	output := badrun(c, 0, "help-tool")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	template := "%v"
	if runtime.GOOS == "windows" {
		template = "%v.exe"
		for i, aCmd := range expectedCommands {
			expectedCommands[i] = fmt.Sprintf(template, aCmd)
		}
	}
	for i, line := range lines {
		command := strings.Fields(line)[0]
		lines[i] = fmt.Sprintf(template, command)
	}
	c.Assert(lines, gc.DeepEquals, expectedCommands)
}

func (suite *HelpToolSuite) TestHelpToolName(c *gc.C) {
	var output string
	if runtime.GOOS == "windows" {
		output = badrun(c, 0, "help-tool", "relation-get.exe")
	} else {
		output = badrun(c, 0, "help-tool", "relation-get")
	}
	expectedHelp := `Usage: relation-get \[options\] <key> <unit id>

Summary:
get relation settings

Options:
(.|\n)*

Details:
relation-get prints the value(.|\n)*`
	c.Assert(output, gc.Matches, expectedHelp)
}
