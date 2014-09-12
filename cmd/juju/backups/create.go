// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const createDoc = `
"create" requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored in juju and
will be lost when the environment is destroyed.
`

var sendCreateRequest = func(cmd *BackupsCreateCommand) (*params.BackupsMetadataResult, error) {
	client, err := cmd.client()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	return client.Create(cmd.Notes)
}

// BackupsCreateCommand is the sub-command for creating a new backup.
type BackupsCreateCommand struct {
	BackupsCommandBase
	Quiet bool
	Notes string
}

func (c *BackupsCreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "[<notes>]",
		Purpose: "create a backup",
		Doc:     createDoc,
	}
}

func (c *BackupsCreateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Quiet, "quiet", false, "do not print the metadata")
}

func (c *BackupsCreateCommand) Init(args []string) error {
	notes, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Notes = notes
	return nil
}

func (c *BackupsCreateCommand) Run(ctx *cmd.Context) error {
	result, err := sendCreateRequest(c)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.Quiet {
		c.dumpMetadata(ctx, result)
	}

	fmt.Fprintln(ctx.Stdout, result.ID)
	return nil
}
