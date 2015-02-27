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

const uploadDoc = `
"upload" sends a backup archive file to remote storage.
`

// UploadCommand is the sub-command for uploading a backup archive.
type UploadCommand struct {
	CommandBase
	// Filename is where to find the archive to upload.
	Filename string
	// ShowMeta indicates that the uploaded metadata should be printed.
	ShowMeta bool
	// Quiet indicates that the new backup ID should not be printed.
	Quiet bool
}

// SetFlags implements Command.SetFlags.
func (c *UploadCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.ShowMeta, "verbose", false, "show the uploaded metadata")
	f.BoolVar(&c.Quiet, "quiet", false, "do not print the new backup ID")
}

// Info implements Command.Info.
func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "<filename>",
		Purpose: "store a backup archive file remotely in juju",
		Doc:     uploadDoc,
	}
}

// Init implements Command.Init.
func (c *UploadCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("backup filename not specified")
	}
	filename, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	c.Filename = filename
	return nil
}

// Run implements Command.Run.
func (c *UploadCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	archive, meta, err := getArchive(c.Filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	if c.ShowMeta {
		fmt.Fprintln(ctx.Stdout, "Uploaded metadata:")
		c.dumpMetadata(ctx, meta)
		fmt.Fprintln(ctx.Stdout)
	}

	// Upload the archive.
	id, err := client.Upload(archive, *meta)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Quiet {
		fmt.Fprintln(ctx.Stdout, id)
		return nil
	}

	// Pull the stored metadata.
	stored, err := c.getStoredMetadata(id)
	if err != nil {
		return errors.Trace(err)
	}

	c.dumpMetadata(ctx, stored)
	return nil
}

func (c *UploadCommand) getStoredMetadata(id string) (*params.BackupsMetadataResult, error) {
	// TODO(ericsnow) lp-1399722 This should be addressed.
	// There is at least anecdotal evidence that we cannot use an API
	// client for more than a single request. So we use a new client
	// for download.
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	stored, err := client.Info(id)
	return stored, errors.Trace(err)
}
