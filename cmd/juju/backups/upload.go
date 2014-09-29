// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups/archive"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/version"
)

const uploadDoc = `
"upload" sends a backup archive file to remote storage.
`

// UploadCommand is the sub-command for uploading a backup archive.
type UploadCommand struct {
	CommandBase
	// Filename is where to find the archive to upload.
	Filename string
}

// Info implements Command.Info.
func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "<filename>",
		Purpose: "push an archive file",
		Doc:     uploadDoc,
	}
}

// Init implements Command.Init.
func (c *UploadCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing filename")
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

	// Upload the archive.
	result, err := client.Upload(archive, *meta)
	if err != nil {
		return errors.Trace(err)
	}

	c.dumpMetadata(ctx, result)
	return nil
}

func (c *UploadCommand) getArchive(filename string) (io.ReadCloser, *params.BackupsMetadataResult, error) {

	archiveFile, err := os.Open(filename)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	meta, err := archive.GetMetadata(archiveFile)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, errors.Trace(err)
		}
		archiveFile.Seek(0, os.SEEK_SET)

		env := ""                // unknown
		machine := ""            // unknown
		hostname := ""           // unknown
		vers := version.Number{} // unknown
		origin := metadata.ExistingOrigin(env, machine, hostname, vers)

		notes := ""
		meta, err = metadata.BuildMetadata(archiveFile, *origin, notes)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	archiveFile.Seek(0, os.SEEK_SET)

	var metaResult params.BackupsMetadataResult
	metaResult.UpdateFromMetadata(meta)

	return archiveFile, &metaResult, nil
}
