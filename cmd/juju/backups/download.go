// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const downloadDoc = `
Retrieves a backup archive file.

If ` + "`--filename`" + ` is not used, the archive is downloaded to a temporary
location and the filename is printed to stdout.
`

const examples = `
    juju download-backup /full/path/to/backup/on/controller
`

// NewDownloadCommand returns a commant used to download backups.
func NewDownloadCommand() cmd.Command {
	return modelcmd.Wrap(&downloadCommand{})
}

// downloadCommand is the sub-command for downloading a backup archive.
type downloadCommand struct {
	CommandBase
	// LocalFilename is where to save the downloaded archive.
	LocalFilename string
	// RemoteFilename is the backup filename to download.
	RemoteFilename string
}

// Info implements Command.Info.
func (c *downloadCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "download-backup",
		Args:     "/full/path/to/backup/on/controller",
		Purpose:  "Download a backup archive file.",
		Doc:      downloadDoc,
		Examples: examples,
		SeeAlso: []string{
			"create-backup",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *downloadCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.LocalFilename, "filename", "", "Download target")
}

// Init implements Command.Init.
func (c *downloadCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing filename")
	}
	filename, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	c.RemoteFilename = filename
	return nil
}

// Run implements Command.Run.
func (c *downloadCommand) Run(ctx *cmd.Context) error {
	if err := c.validateIaasController(c.Info().Name); err != nil {
		return errors.Trace(err)
	}
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	// Download the archive.
	resultArchive, err := client.Download(c.RemoteFilename)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			ctx.Errorf("Download of backup archive files is not supported by this controller.")
			return nil
		}
		return errors.Trace(err)
	}
	defer func() { _ = resultArchive.Close() }()

	// Prepare the local archive.
	filename := c.ResolveFilename()
	archive, err := c.Filesystem().Create(filename)
	if err != nil {
		return errors.Annotate(err, "while creating local archive file")
	}
	defer func() { _ = archive.Close() }()

	// Write out the archive.
	_, err = io.Copy(archive, resultArchive)
	if err != nil {
		return errors.Annotate(err, "while copying local archive file")
	}

	// Print the local filename.
	fmt.Fprintln(ctx.Stdout, filename)
	return nil
}

// ResolveFilename returns the filename used by the command.
func (c *downloadCommand) ResolveFilename() string {
	filename := c.LocalFilename
	if filename == "" {
		_, filename = filepath.Split(c.RemoteFilename)
	}
	return filename
}
