// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// KillCommand removes an Action from the pending queue
type KillCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const killDoc = `
Remove an Action from the queue of pending Actions by ID.

ex.

juju action kill action:UUID
`

func (c *KillCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "kill",
		Args:    "<action identifier>",
		Purpose: "TODO: remove an action from the queue",
		Doc:     killDoc,
	}
}
