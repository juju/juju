// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.machine")

const commandDoc = `
"juju environment" provides commands to interact with the Juju environment.
`

const commandPurpose = "manage environments"

// NewSuperCommand creates the environment supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	environmentCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "environment",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     commandPurpose,
	})
	environmentCmd.Register(envcmd.Wrap(&GetCommand{}))
	environmentCmd.Register(envcmd.Wrap(&SetCommand{}))
	environmentCmd.Register(envcmd.Wrap(&UnsetCommand{}))
	return environmentCmd
}
