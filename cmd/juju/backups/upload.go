// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

const uploadDoc = `
"upload" sends a backup archive file to remote storage.
`

func newUploadCommand() cmd.Command {
	return modelcmd.Wrap(&uploadCommand{})
}

// uploadCommand is the sub-command for uploading a backup archive.
type uploadCommand struct {
	CommandBase
	// Filename is where to find the archive to upload.
	Filename string
}

// Info implements Command.Info.
func (c *uploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "<filename>",
		Purpose: "store a backup archive file remotely in juju",
		Doc:     uploadDoc,
	}
}

// Init implements Command.Init.
func (c *uploadCommand) Init(args []string) error {
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
func (c *uploadCommand) Run(ctx *cmd.Context) error {
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

	archive, meta, err := getArchive(c.Filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	if c.Log != nil && c.Log.Verbose {
		fmt.Fprintln(ctx.Stdout, "Uploaded metadata:")
		c.dumpMetadata(ctx, meta)
		fmt.Fprintln(ctx.Stdout)
	}

	// Upload the archive.
	id, err := client.Upload(archive, *meta)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Log != nil && c.Log.Quiet {
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

func (c *uploadCommand) getStoredMetadata(id string) (*params.BackupsMetadataResult, error) {
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
