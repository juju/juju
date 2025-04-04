// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	"github.com/juju/juju/internal/testing"
	gc "gopkg.in/check.v1"
)

type HelpActionCommandsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&HelpActionCommandsSuite{})

func (suite *HelpActionCommandsSuite) SetUpTest(c *gc.C) {
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	setFeatureFlags("")
}

func (suite *HelpActionCommandsSuite) TestHelpActionCommandsHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-action-commands")
	c.Assert(output, gc.Equals, `Usage: juju help-action-commands [action]

Summary:
Show help on a Juju charm action command.

Global Options:
--debug  (= false)
    Equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    Specify log levels for modules
--quiet  (= false)
    Show no informational output
--show-log  (= false)
    If set, write the log file to stderr
--verbose  (= false)
    Show more verbose output

Details:
In addition to hook commands, Juju charms also have access to a set of action-specific commands. 
These action commands are available when an action is running, and are used to log progress
and report the outcome of the action.
The currently available charm action commands include:
    action-fail  Set action fail status with message.
    action-get   Get action parameters.
    action-log   Record a progress message for the current action.
    action-set   Set action results.

Examples:

For help on a specific action command, supply the name of that action command, for example:

        juju help-action-commands action-fail

See also:
 - help
 - help-hook-commands
`)
}

var expectedActionCommands = []string{
	"action-fail",
	"action-get",
	"action-log",
	"action-set",
}

func (suite *HelpActionCommandsSuite) TestHelpActionCommands(c *gc.C) {
	output := badrun(c, 0, "help-action-commands")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		command := strings.Fields(line)[0]
		lines[i] = command
	}
	c.Assert(lines, gc.DeepEquals, expectedActionCommands)
}

func (suite *HelpActionCommandsSuite) TestHelpActionCommandsName(c *gc.C) {
	output := badrun(c, 0, "help-action-commands", "action-fail")
	expectedHelp := `Usage: action-fail ["<failure message>"]

Summary:
Set action fail status with message.

Details:
action-fail sets the fail state of the action with a given error message.  Using
action-fail without a failure message will set a default message indicating a
problem with the action.

Examples:

    action-fail 'unable to contact remote service'
`
	c.Assert(output, gc.DeepEquals, expectedHelp)
}
