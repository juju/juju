// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/testing"
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

    action-fail              Set action fail status with message.
    action-get               Get action parameters.
    action-log               Record a progress message for the current action.
    action-set               Set action results.
    application-version-set  Specify which version of the application is deployed.
    close-port               Register a request to close a port or port range.
    config-get               Print application configuration.
    credential-get           Access cloud credentials.
    goal-state               Print the status of the charm's peers and related units.
    is-leader                Print application leadership status.
    juju-log                 Write a message to the juju log.
    juju-reboot              Reboot the host machine.
    leader-get               Print application leadership settings.
    leader-set               Write application leadership settings.
    network-get              Get network config.
    open-port                Register a request to open a port or port range.
    opened-ports             List all ports or port ranges opened by the unit.
    payload-register         Register a charm payload with Juju.
    payload-status-set       Update the status of a payload.
    payload-unregister       Stop tracking a payload.
    relation-get             Get relation settings.
    relation-ids             List all relation IDs for the given endpoint.
    relation-list            List relation units.
    relation-set             Set relation settings.
    resource-get             Get the path to the locally cached resource file.
    secret-add               Add a new secret.
    secret-get               Get the content of a secret.
    secret-grant             Grant access to a secret.
    secret-ids               Print secret IDs.
    secret-info-get          Get a secret's metadata info.
    secret-remove            Remove an existing secret.
    secret-revoke            Revoke access to a secret.
    secret-set               Update an existing secret.
    state-delete             Delete server-side-state key value pairs.
    state-get                Print server-side-state value.
    state-set                Set server-side-state values.
    status-get               Print status information.
    status-set               Set status information.
    storage-add              Add storage instances.
    storage-get              Print information for the storage instance with the specified ID.
    storage-list             List storage attached to the unit.
    unit-get                 Print public-address or private-address.

Examples:

For help on a specific tool, supply the name of that tool, for example:

        juju help-tool unit-get

See also:
 - help
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
Get relation settings.

Options:
(.|\n)*

Details:
relation-get prints the value(.|\n)*`
	c.Assert(output, gc.Matches, expectedHelp)
}
