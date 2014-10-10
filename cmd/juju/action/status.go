// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// StatusCommand shows the status of an Action by ID.
type StatusCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const statusDoc = `
Show the status of an Action by identifier.
`

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "<action id>",
		Purpose: "TODO: show status of action by id",
		Doc:     statusDoc,
	}
}
