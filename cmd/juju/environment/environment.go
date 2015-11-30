// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
)

var logger = loggo.GetLogger("juju.cmd.juju.environment")

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
	environmentCmd.Register(newGetCommand())
	environmentCmd.Register(newSetCommand())
	environmentCmd.Register(newUnsetCommand())
	environmentCmd.Register(&JenvCommand{})
	environmentCmd.Register(newRetryProvisioningCommand())
	environmentCmd.Register(newEnvSetConstraintsCommand())
	environmentCmd.Register(newEnvGetConstraintsCommand())

	if featureflag.Enabled(feature.JES) {
		environmentCmd.Register(newShareCommand())
		environmentCmd.Register(newUnshareCommand())
		environmentCmd.Register(newUsersCommand())
		environmentCmd.Register(newDestroyCommand())
	}
	return environmentCmd
}
