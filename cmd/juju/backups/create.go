// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const createDoc = `
"create" requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored in juju and
will be lost when the environment is destroyed.
`

// CreateCommand is the sub-command for creating a new backup.
type CreateCommand struct {
	CommandBase
	// Quiet indicates that the full metadata should not be dumped.
	Quiet bool
	// Notes is the custom message to associated with the new backup.
	Notes string
}

// Info implements Command.Info.
func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "[<notes>]",
		Purpose: "create a backup",
		Doc:     createDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *CreateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Quiet, "quiet", false, "do not print the metadata")
}

// Init implements Command.Init.
func (c *CreateCommand) Init(args []string) error {
	notes, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Notes = notes
	return nil
}

// Run implements Command.Run.
func (c *CreateCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	result, err := client.Create(c.Notes)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.Quiet {
		c.dumpMetadata(ctx, result)
	}

	fmt.Fprintln(ctx.Stdout, result.ID)
	return nil
}
