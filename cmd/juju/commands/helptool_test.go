// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/testing"
)

type HelpToolSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&HelpToolSuite{})

func (suite *HelpToolSuite) SetUpTest(c *gc.C) {
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	setFeatureFlags(feature.Secrets)
}

func (suite *HelpToolSuite) TestHelpToolHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-tool")
	c.Assert(output, gc.Equals, `Usage: juju hook-tool [tool]

Summary:
Show help on a Juju charm hook tool.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Details:
Juju charms can access a series of built-in helpers called 'hook-tools'.
These are useful for the charm to be able to inspect its running environment.
Currently available charm hook tools are:

    action-fail              set action fail status with message
    action-get               get action parameters
    action-log               record a progress message for the current action
    action-set               set action results
    add-metric               add metrics
    application-version-set  specify which version of the application is deployed
    close-port               register a request to close a port or port range
    config-get               print application configuration
    credential-get           access cloud credentials
    goal-state               print the status of the charm's peers and related units
    is-leader                print application leadership status
    juju-log                 write a message to the juju log
    juju-reboot              Reboot the host machine
    k8s-raw-get              get k8s raw spec information
    k8s-raw-set              set k8s raw spec information
    k8s-spec-get             get k8s spec information
    k8s-spec-set             set k8s spec information
    leader-get               print application leadership settings
    leader-set               write application leadership settings
    network-get              get network config
    open-port                register a request to open a port or port range
    opened-ports             list all ports or port ranges opened by the unit
    payload-register         register a charm payload with juju
    payload-status-set       update the status of a payload
    payload-unregister       stop tracking a payload
    pod-spec-get             get k8s spec information (deprecated)
    pod-spec-set             set k8s spec information (deprecated)
    relation-get             get relation settings
    relation-ids             list all relation ids with the given relation name
    relation-list            list relation units
    relation-set             set relation settings
    resource-get             get the path to the locally cached resource file
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

        juju hook-tool unit-get

Aliases: help-tool, hook-tools
`)
}

var expectedCommands = []string{
	"action-fail",
	"action-get",
	"action-log",
	"action-set",
	"add-metric",
	"application-version-set",
	"close-port",
	"config-get",
	"credential-get",
	"goal-state",
	"is-leader",
	"juju-log",
	"juju-reboot",
	"k8s-raw-get",
	"k8s-raw-set",
	"k8s-spec-get",
	"k8s-spec-set",
	"leader-get",
	"leader-set",
	"network-get",
	"open-port",
	"opened-ports",
	"payload-register",
	"payload-status-set",
	"payload-unregister",
	"pod-spec-get",
	"pod-spec-set",
	"relation-get",
	"relation-ids",
	"relation-list",
	"relation-set",
	"resource-get",
	"secret-create",
	"secret-get",
	"secret-grant",
	"secret-revoke",
	"secret-update",
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
