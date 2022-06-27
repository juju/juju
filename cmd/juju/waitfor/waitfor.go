// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/loggo"

	_ "github.com/juju/juju/provider/all"
)

var logger = loggo.GetLogger("juju.plugins.waitfor")

var waitForDoc = `
Waits for a specified model, machine, application or unit to reach a state
defined by the supplied query.
`

// NewWaitForCommand creates the wait-for supercommand and registers the
// subcommands that it supports.
func NewWaitForCommand() cmd.Command {
	waitFor := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "wait-for",
		UsagePrefix: "juju",
		Doc:         waitForDoc,
		Purpose:     "Wait for an entity to reach a specified state."})

	waitFor.Register(newApplicationCommand())
	waitFor.Register(newMachineCommand())
	waitFor.Register(newModelCommand())
	waitFor.Register(newUnitCommand())
	return waitFor
}
