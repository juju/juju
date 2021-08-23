// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const listDoc = `
backups provides the metadata associated with all backups.
`

// NewListCommand returns a command used to list metadata for backups.
func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

// listCommand is the sub-command for listing all available backups.
type listCommand struct {
	CommandBase
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "backups",
		Args:    "",
		Purpose: "Displays information about all backups.",
		Doc:     listDoc,
		Aliases: []string{"list-backups"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) error {
	if err := c.CommandBase.Init(args); err != nil {
		return err
	}
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) error {
	if err := c.validateIaasController(c.Info().Name); err != nil {
		return errors.Trace(err)
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
		ctx.Infof("No backups to display.")
		return nil
	}

	for _, resultItem := range result.List {
		if !c.verbose {
			fmt.Fprintln(ctx.Stdout, resultItem.ID)
			continue
		}
		c.dumpMetadata(ctx, &resultItem)
	}
	return nil
}
