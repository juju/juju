// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.machine")

const machineCommandDoc = `
"juju machine" provides commands to add and remove machines in the Juju environment.
`

const machineCommandPurpose = "manage machines"

// NewSuperCommand creates the user supercommand and registers the subcommands
// that it supports.
func NewSuperCommand() cmd.Command {
	machineCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "machine",
		Doc:         machineCommandDoc,
		UsagePrefix: "juju",
		Purpose:     machineCommandPurpose,
	})
	machineCmd.Register(envcmd.Wrap(&AddCommand{}))
	machineCmd.Register(envcmd.Wrap(&RemoveCommand{}))
	return machineCmd
}
