// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const listDoc = `
"list" provides the metadata associated with all backups.
`

// ListCommand is the sub-command for listing all available backups.
type ListCommand struct {
	CommandBase
	// Brief means only IDs will be printed.
	Brief bool
}

// Info implements Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Args:    "",
		Purpose: "get all metadata",
		Doc:     listDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Brief, "brief", false, "only print IDs")
}

// Init implements Command.Init.
func (c *ListCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) error {
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

	if c.Brief {
		fmt.Fprintln(ctx.Stdout, result.List[0].ID)
	} else {
		c.dumpMetadata(ctx, &result.List[0])
	}
	for _, resultItem := range result.List[1:] {
		if c.Brief {
			fmt.Fprintln(ctx.Stdout, resultItem.ID)
		} else {
			fmt.Fprintln(ctx.Stdout)
			c.dumpMetadata(ctx, &resultItem)
		}
	}
	return nil
}
