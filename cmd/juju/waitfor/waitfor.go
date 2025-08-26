// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"github.com/juju/cmd/v3"
)

// Logger is the interface used by the wait-for command to log messages.
type Logger interface {
	Infof(string, ...any)
	Verbosef(string, ...any)
}

var waitForDoc = `
The ` + "`wait-for`" + ` set of commands (model, application, machine and unit) defines
a way to wait for a goal state to be reached. The goal state can be defined
programmatically using the query DSL (domain specific language).

The ` + "`wait-for`" + ` command is an optimized alternative to the ` + "`status`" + ` command for
determining programmatically if a goal state has been reached. The ` + "`wait-for`" + `
command streams delta changes from the underlying database, unlike the ` + "`status`" + `
command which performs a full query of the database.

The query DSL is a simple language that can be comprised of expressions to
produce a Boolean result. The result of the query is used to determine if the
goal state has been reached. The query DSL is evaluated against the scope of
the command.

Built-in functions are provided to help define the goal state. The built-in
functions are defined in the ` + "`query`" + ` package. Examples of built-in functions
include ` + "`len`" + `, ` + "`print`" + `, ` + "`forEach`" + ` (` + "`lambda`" + `), ` + "`startsWith`" + ` and ` + "`endsWith`" + `.

See also:

    wait-for model
    wait-for application
    wait-for machine
    wait-for unit
`

const waitForExamples = `
Waits for the ` + "`mysql/0`" + ` unit to be created and active.

    juju wait-for unit mysql/0

Waits for the ` + "`mysql`" + ` application to be active or idle.

    juju wait-for application mysql --query='name=="mysql" && (status=="active" || status=="idle")'

Waits for the model units to all start with ` + "`ubuntu`" + `.

    juju wait-for model default --query='forEach(units, unit => startsWith(unit.name, "ubuntu"))'
`

// NewWaitForCommand creates the wait-for supercommand and registers the
// subcommands that it supports.
func NewWaitForCommand() cmd.Command {
	waitFor := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "wait-for",
		UsagePrefix: "juju",
		Doc:         waitForDoc,
		Purpose:     "Wait for an entity to reach a specified state.",
		Examples:    waitForExamples,
	})

	waitFor.Register(newApplicationCommand())
	waitFor.Register(newMachineCommand())
	waitFor.Register(newModelCommand())
	waitFor.Register(newUnitCommand())
	return waitFor
}
