// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type HelpHookCommandsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&HelpHookCommandsSuite{})

func (suite *HelpHookCommandsSuite) SetUpTest(c *gc.C) {
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	setFeatureFlags("")
}

func (suite *HelpHookCommandsSuite) TestHelpHookCommandsHelp(c *gc.C) {
	output := badrun(c, 0, "help", "help-hook-commands")
	c.Assert(output, gc.Equals, `Usage: juju help-hook-commands [hook]

Summary:
Show help on a Juju charm hook command.

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
Juju charms have access to a set of built-in helpers known as 'hook-commands,'
which allow them to inspect their runtime environment.
The currently available charm hook commands include:

    add-metric               Add metrics.
    application-version-set  Specify which version of the application is deployed.
    close-port               Register a request to close a port or port range.
    config-get               Print application configuration.
    credential-get           Access cloud credentials.
    goal-state               Print the status of the charm's peers and related units.
    is-leader                Print application leadership status.
    juju-log                 Write a message to the juju log.
    juju-reboot              Reboot the host machine.
    k8s-raw-get              Get k8s raw spec information.
    k8s-raw-set              Set k8s raw spec information.
    k8s-spec-get             Get k8s spec information.
    k8s-spec-set             Set k8s spec information.
    leader-get               Print application leadership settings.
    leader-set               Write application leadership settings.
    network-get              Get network config.
    open-port                Register a request to open a port or port range.
    opened-ports             List all ports or port ranges opened by the unit.
    payload-register         Register a charm payload with Juju.
    payload-status-set       Update the status of a payload.
    payload-unregister       Stop tracking a payload.
    pod-spec-get             Get k8s spec information. (deprecated)
    pod-spec-set             Set k8s spec information. (deprecated)
    relation-get             Get relation settings.
    relation-ids             List all relation IDs for the given endpoint.
    relation-list            List relation units.
    relation-model-get       Get details about the model hosing a related application.
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

For help on a specific hook command, supply the name of that hook command, for example:

    juju help-hook-commands unit-get

See also:
 - help
 - help-action-commands
`)
}

var expectedCommands = []string{
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
	"relation-model-get",
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

func (suite *HelpHookCommandsSuite) TestHelpHookCommands(c *gc.C) {
	output := badrun(c, 0, "help-hook-commands")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		command := strings.Fields(line)[0]
		lines[i] = command
	}
	c.Assert(lines, gc.DeepEquals, expectedCommands)
}

func (suite *HelpHookCommandsSuite) TestHelpHookCommandsName(c *gc.C) {
	output := badrun(c, 0, "help-hook-commands", "relation-get")
	expectedHelp := `Usage: relation-get \[options\] <key> <unit id>

Summary:
Get relation settings.

Options:
(.|\n)*

Details:
relation-get prints the value(.|\n)*`
	c.Assert(output, gc.Matches, expectedHelp)
}
