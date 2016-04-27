// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

const showDoc = `
show-backup provides the metadata associated with a backup.
`

// NewShowCommand returns a command used to show metadata for a backup.
func NewShowCommand() cmd.Command {
	return modelcmd.Wrap(&showCommand{})
}

// showCommand is the sub-command for creating a new backup.
type showCommand struct {
	CommandBase
	// ID is the backup ID to get.
	ID string
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-backup",
		Args:    "<ID>",
		Purpose: "get metadata",
		Doc:     showDoc,
	}
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) error {
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
func (c *showCommand) Run(ctx *cmd.Context) error {
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

	result, err := client.Info(c.ID)
	if err != nil {
		return errors.Trace(err)
	}

	c.dumpMetadata(ctx, result)
	return nil
}
