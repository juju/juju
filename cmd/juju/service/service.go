// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.juju.service")

const commandDoc = `
"juju service" provides commands to manage Juju services.
`

// NewSuperCommand creates the service supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	environmentCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "service",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     "manage services",
	})

	environmentCmd.Register(newAddUnitCommand())
	environmentCmd.Register(newServiceGetConstraintsCommand())
	environmentCmd.Register(newServiceSetConstraintsCommand())
	environmentCmd.Register(newGetCommand())
	environmentCmd.Register(NewSetCommand())
	environmentCmd.Register(newUnsetCommand())

	return environmentCmd
}
