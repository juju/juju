// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type HelpToolSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&HelpToolSuite{})

func (suite *HelpToolSuite) SetUpTest(c *gc.C) {
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	setFeatureFlags("")
}

func (suite *HelpToolSuite) TestHelpToolHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-tool")
	c.Assert(output, gc.Equals, `Usage: juju help-tool [tool]

Summary:
Show help on a Juju charm hook tool.

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
Juju charms can access a series of built-in helpers called 'hook-tools'.
These are useful for the charm to be able to inspect its running environment.
Currently available charm hook tools are:

    action-fail              set action fail status with message
    action-get               get action parameters
    action-log               record a progress message for the current action
    action-set               set action results
    application-version-set  specify which version of the application is deployed
    close-port               register a request to close a port or port range
    config-get               print application configuration
    credential-get           access cloud credentials
    goal-state               print the status of the charm's peers and related units
    is-leader                print application leadership status
    juju-log                 write a message to the juju log
    juju-reboot              Reboot the host machine
    leader-get               print application leadership settings
    leader-set               write application leadership settings
    network-get              get network config
    open-port                register a request to open a port or port range
    opened-ports             list all ports or port ranges opened by the unit
    payload-register         register a charm payload with juju
    payload-status-set       update the status of a payload
    payload-unregister       stop tracking a payload
    relation-get             get relation settings
    relation-ids             list all relation ids for the given endpoint
    relation-list            list relation units
    relation-set             set relation settings
    resource-get             get the path to the locally cached resource file
    secret-add               add a new secret
    secret-get               get the content of a secret
    secret-grant             grant access to a secret
    secret-ids               print secret ids
    secret-info-get          get a secret's metadata info
    secret-remove            remove a existing secret
    secret-revoke            revoke access to a secret
    secret-set               update an existing secret
    state-delete             delete server-side-state key value pair
    state-get                print server-side-state value
    state-set                set server-side-state values
    status-get               print status information
    status-set               set status information
    storage-add              add storage instances
    storage-get              print information for storage instance with specified id
    storage-list             list storage attached to the unit
    unit-get                 print public-address or private-address

Examples:

For help on a specific tool, supply the name of that tool, for example:

        juju help-tool unit-get
`)
}

var expectedCommands = []string{
	"action-fail",
	"action-get",
	"action-log",
	"action-set",
	"application-version-set",
	"close-port",
	"config-get",
	"credential-get",
	"goal-state",
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
	"secret-add",
	"secret-get",
	"secret-grant",
	"secret-ids",
	"secret-info-get",
	"secret-remove",
	"secret-revoke",
	"secret-set",
	"state-delete",
	"state-get",
	"state-set",
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
	for i, line := range lines {
		command := strings.Fields(line)[0]
		lines[i] = command
	}
	c.Assert(lines, gc.DeepEquals, expectedCommands)
}

func (suite *HelpToolSuite) TestHelpToolName(c *gc.C) {
	var output string
	output = badrun(c, 0, "help-tool", "relation-get")
	expectedHelp := `Usage: relation-get \[options\] <key> <unit id>

Summary:
get relation settings

Options:
(.|\n)*

Details:
relation-get prints the value(.|\n)*`
	c.Assert(output, gc.Matches, expectedHelp)
}
