// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// WaitCommand waits for the results of an Action by ID.
type WaitCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const waitDoc = `
Wait for the results of an Action by ID.
`

func (c *WaitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "wait",
		Args:    "<action ID>",
		Purpose: "TODO: wait for results of an action",
		Doc:     waitDoc,
	}
}
