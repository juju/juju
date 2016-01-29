// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

const infoDoc = `
"info" provides the metadata associated with a backup.
`

func newInfoCommand() cmd.Command {
	return modelcmd.Wrap(&infoCommand{})
}

// infoCommand is the sub-command for creating a new backup.
type infoCommand struct {
	CommandBase
	// ID is the backup ID to get.
	ID string
}

// Info implements Command.Info.
func (c *infoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "info",
		Args:    "<ID>",
		Purpose: "get metadata",
		Doc:     infoDoc,
	}
}

// Init implements Command.Init.
func (c *infoCommand) Init(args []string) error {
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
func (c *infoCommand) Run(ctx *cmd.Context) error {
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
