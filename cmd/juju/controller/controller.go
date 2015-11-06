// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.juju.controller")

const commandDoc = `

A Juju controller is a Juju environment that runs the API servers, and manages
the underlying database used by Juju. The initial environment that is created
when bootstrapping is called a "controller".

The "juju controller" command provides the commands to create, use, and destroy
environments running withing a Juju controller.

Controller commands also allow the user to connect to an existing controller
using the "login" command, and to use an environment that already exists in
the current controller through the "use-environment" command.

see also:
    juju help juju-controllers
`

// NewSuperCommand creates the controller supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	controllerCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "controller",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     "manage controllers and their hosted environments",
	})

	controllerCmd.Register(newListCommand())
	controllerCmd.Register(newLoginCommand())
	controllerCmd.Register(newDestroyCommand())
	controllerCmd.Register(newKillCommand())
	controllerCmd.Register(newListBlocksCommand())
	controllerCmd.Register(newEnvironmentsCommand())
	controllerCmd.Register(newCreateEnvironmentCommand())
	controllerCmd.Register(newRemoveBlocksCommand())
	controllerCmd.Register(newUseEnvironmentCommand())

	return controllerCmd
}
