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

// NewSuperCommand creates the environment supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	environmentCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "environment",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     "manage environments",
	})
	environmentCmd.Register(envcmd.Wrap(&GetCommand{}))
	environmentCmd.Register(envcmd.Wrap(&SetCommand{}))
	environmentCmd.Register(envcmd.Wrap(&UnsetCommand{}))
	environmentCmd.Register(&JenvCommand{})
	environmentCmd.Register(envcmd.Wrap(&EnsureAvailabilityCommand{}))
	environmentCmd.Register(envcmd.Wrap(&RetryProvisioningCommand{}))
	return environmentCmd
}
