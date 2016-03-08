// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

const removeDoc = `
"remove" removes a backup from remote storage.
`

func newRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

type removeCommand struct {
	CommandBase
	// ID refers to the backup to be removed.
	ID string
}

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<ID>",
		Purpose: "delete a backup",
		Doc:     removeDoc,
	}
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) error {
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
func (c *removeCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	err = client.Remove(c.ID)
	if err != nil {
		return errors.Trace(err)
	}

	output := fmt.Sprintf("successfully removed: %v\n", c.ID)
	ctx.Stdout.Write([]byte(output))
	return nil
}
