// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// QueueCommand shows the queue of pending Actions.
type QueueCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const queueDoc = `
Show the queue of pending Actions and their identifiers.
`

func (c *QueueCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "queue",
		Purpose: "TODO: show queued actions",
		Doc:     queueDoc,
	}
}
