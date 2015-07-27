// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
)

var logger = loggo.GetLogger("juju.cmd.juju.system")

const commandDoc = `

A Juju system is a Juju environment that runs the API servers, and manages the
underlying database used by Juju. The initial environment that is created when
bootstrapping is called a "system".

The "juju system" command provides the commands to create, use, and destroy
environments running withing a Juju system.

System commands also allow the user to connect to an existing system using the
"login" command, and to use an environment that already exists in the current
system through the "use-environment" command.

see also:
    juju help juju-systems
`

// NewSuperCommand creates the system supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	systemCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "system",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     "manage systems",
	})

	systemCmd.Register(&ListCommand{})
	systemCmd.Register(&LoginCommand{})
	systemCmd.Register(&DestroyCommand{})
	systemCmd.Register(&KillCommand{apiDialerFunc: juju.NewAPIFromName})
	systemCmd.Register(envcmd.WrapSystem(&ListBlocksCommand{}))
	systemCmd.Register(envcmd.WrapSystem(&EnvironmentsCommand{}))
	systemCmd.Register(envcmd.WrapSystem(&CreateEnvironmentCommand{}))
	systemCmd.Register(envcmd.WrapSystem(&RemoveBlocksCommand{}))
	systemCmd.Register(envcmd.WrapSystem(&UseEnvironmentCommand{}))

	return systemCmd
}
