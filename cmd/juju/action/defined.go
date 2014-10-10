// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// DefinedCommand lists actions defined by the charm of a given service.
type DefinedCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const definedDoc = `
Show the actions available to run on the target service, with a short
description.  To show the schema for the actions, use --schema.

For more information, see also the 'do' subcommand, which executes actions.
`

func (c *DefinedCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "defined",
		Args:    "<service name> [--schema]",
		Purpose: "TODO: show actions defined for a service",
		Doc:     definedDoc,
	}
}
