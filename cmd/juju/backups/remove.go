// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

const removeDoc = `
"remove" removes a backup from remote storage.
`

// CreateCommand is the sub-command for creating a new backup.
type RemoveCommand struct {
	CommandBase
	// ID refers to the backup to be removed.
	ID string
}

// Info implements Command.Info.
func (c *RemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<ID>",
		Purpose: "delete a backup",
		Doc:     removeDoc,
	}
}

// Init implements Command.Init.
func (c *RemoveCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing ID")
	}
	id, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	c.ID = id
	return nil
}

// Run implements Command.Run.
func (c *RemoveCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	err = client.Remove(c.ID)
	if err != nil {
		return errors.Trace(err)
	}

	fmt.Fprintln(ctx.Stdout, "successfully removed:", c.ID)
	return nil
}
