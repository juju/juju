// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
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

func (suite *HelpToolSuite) TestHelpTool(c *gc.C) {
	expectedNames := jujuc.CommandNames()
	output := badrun(c, 0, "help-tool")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	template := "%v"
	if runtime.GOOS == "windows" {
		template = "%v.exe"
	}
	for i, line := range lines {
		command := strings.Fields(line)[0]
		lines[i] = fmt.Sprintf(template, command)
	}
	c.Assert(lines, gc.DeepEquals, expectedNames)
}

// Component-based features such as payloads and resources
// are different enough in implementation to the rest
// of Juju code that we need to ensure that help-tool can reach them
// explicitely.
func (suite *HelpToolSuite) TestHelpToolHasComponents(c *gc.C) {
	hasPayloads, hasResources := false, false
	output := badrun(c, 0, "help-tool")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		command := strings.Fields(line)[0]
		if strings.HasPrefix(command, "payload-") {
			hasPayloads = true
		}
		if strings.HasPrefix(command, "resource-") {
			hasResources = true
		}
	}
	c.Assert(hasPayloads, jc.IsTrue)
	c.Assert(hasResources, jc.IsTrue)
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
