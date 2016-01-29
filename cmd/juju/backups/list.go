// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

const listDoc = `
"list" provides the metadata associated with all backups.
`

func newListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

// listCommand is the sub-command for listing all available backups.
type listCommand struct {
	CommandBase
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Args:    "",
		Purpose: "get all metadata",
		Doc:     listDoc,
	}
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) error {
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

	result, err := client.List()
	if err != nil {
		return errors.Trace(err)
	}

	if len(result.List) == 0 {
		fmt.Fprintln(ctx.Stdout, "(no backups found)")
		return nil
	}

	verbose := c.Log != nil && c.Log.Verbose
	if verbose {
		c.dumpMetadata(ctx, &result.List[0])
	} else {
		fmt.Fprintln(ctx.Stdout, result.List[0].ID)
	}
	for _, resultItem := range result.List[1:] {
		if verbose {
			fmt.Fprintln(ctx.Stdout)
			c.dumpMetadata(ctx, &resultItem)
		} else {
			fmt.Fprintln(ctx.Stdout, resultItem.ID)
		}
	}
	return nil
}
