// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.system")

const commandDoc = `
"juju system" provides commands to manage Juju systems.
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
	systemCmd.Register(envcmd.WrapSystem(&EnvironmentsCommand{}))
	systemCmd.Register(envcmd.WrapSystem(&UseEnvironmentCommand{}))

	return systemCmd
}
