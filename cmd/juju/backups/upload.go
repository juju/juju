// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups"
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

	archive, meta, err := c.getArchive(c.Filename)
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
	stored, err := client.Info(id)
	if err != nil {
		return errors.Trace(err)
	}

	c.dumpMetadata(ctx, stored)
	return nil
}

func (c *UploadCommand) getArchive(filename string) (io.ReadCloser, *params.BackupsMetadataResult, error) {

	archive, err := os.Open(filename)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Extract the metadata.
	ad, err := backups.NewArchiveDataReader(archive)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	archive.Seek(0, os.SEEK_SET)
	meta, err := ad.Metadata()
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, errors.Trace(err)
		}
		meta, err = backups.BuildMetadata(archive)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	} else {
		// Make sure the file info is set.
		fileMeta, err := backups.BuildMetadata(archive)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		if meta.Size() == int64(0) {
			if err := meta.SetFileInfo(fileMeta.Size(), "", ""); err != nil {
				return nil, nil, errors.Trace(err)
			}
		}
		if meta.Checksum() == "" {
			err := meta.SetFileInfo(0, fileMeta.Checksum(), fileMeta.ChecksumFormat())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
		}
		if meta.Finished == nil || meta.Finished.IsZero() {
			meta.Finished = fileMeta.Finished
		}
	}
	archive.Seek(0, os.SEEK_SET)

	// Pack the metadata into a result.
	var metaResult params.BackupsMetadataResult
	metaResult.UpdateFromMetadata(meta)

	return archive, &metaResult, nil
}
