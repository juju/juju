// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// LogCommand fetches the log of Action results.
type LogCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const logDoc = `
Fetch the log of Action results.
`

func (c *LogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "log",
		Args:    "",
		Purpose: "TODO: fetch logged action results",
		Doc:     logDoc,
	}
}
