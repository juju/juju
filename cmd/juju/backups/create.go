// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/state/backups"
)

const (
	notset          = backups.FilenamePrefix + "<date>-<time>.tar.gz"
	downloadWarning = "downloading backup archives is recommended; " +
		"backups stored remotely are not guaranteed to be available"
)

const createDoc = `
create-backup requests that Juju creates a backup of its state and prints the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored remotely by Juju, but
will also be copied locally unless --no-download is supplied. To access the
remote backups, see 'juju download-backup'.

See also:
    backups
    download-backup
`

// NewCreateCommand returns a command used to create backups.
func NewCreateCommand() cmd.Command {
	return modelcmd.Wrap(&createCommand{})
}

// createCommand is the sub-command for creating a new backup.
type createCommand struct {
	CommandBase
	// NoDownload means the backups archive should not be downloaded.
	NoDownload bool
	// Filename is where the backup should be downloaded.
	Filename string
	// Notes is the custom message to associated with the new backup.
	Notes string
	// KeepCopy means the backup archive should be stored in the controller db.
	KeepCopy bool
}

// Info implements Command.Info.
func (c *createCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-backup",
		Args:    "[<notes>]",
		Purpose: "Create a backup.",
		Doc:     createDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.NoDownload, "no-download", false, "Do not download the archive, implies keep-copy")
	f.BoolVar(&c.KeepCopy, "keep-copy", false, "Keep a copy of the archive on the controller")
	f.StringVar(&c.Filename, "filename", notset, "Download to this file")
}

// Init implements Command.Init.
func (c *createCommand) Init(args []string) error {
	notes, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Notes = notes

	if c.Filename != notset && c.NoDownload {
		return errors.Errorf("cannot mix --no-download and --filename")
	}

	if c.Filename == "" {
		return errors.Errorf("missing filename")
	}
	return nil
}

// Run implements Command.Run.
func (c *createCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}

	if c.NoDownload {
		ctx.Warningf(downloadWarning)
	}

	client, apiVersion, err := c.NewGetAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if apiVersion < 2 {
		if c.KeepCopy {
			return errors.New("--keep-copy is not supported by this controller")
		}
		// for API v1, keepCopy is the default and only choice, so set it here
		c.KeepCopy = true
	}

	if c.NoDownload {
		c.KeepCopy = true
	}

	metadataResult, copyFrom, err := c.create(client, apiVersion)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO: (hml) 2018-04-25
	// fix to dump the metadata when --verbose used
	if c.Log != nil && !c.Log.Quiet {
		c.dumpMetadata(ctx, metadataResult)
	}

	if c.KeepCopy {
		ctx.Infof(metadataResult.ID)
	}

	// Handle download.
	if !c.NoDownload {
		filename := c.decideFilename(ctx, c.Filename, metadataResult.Started)
		if err := c.download(ctx, client, copyFrom, filename); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *createCommand) decideFilename(ctx *cmd.Context, filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	// Downloading but no filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

func (c *createCommand) download(ctx *cmd.Context, client APIClient, copyFrom string, archiveFilename string) error {
	ctx.Infof("downloading to " + archiveFilename)

	resultArchive, err := client.Download(copyFrom)
	if err != nil {
		return errors.Trace(err)
	}
	defer resultArchive.Close()

	archive, err := os.Create(archiveFilename)
	if err != nil {
		return errors.Annotate(err, "while creating local archive file")
	}
	defer archive.Close()

	_, err = io.Copy(archive, resultArchive)
	return errors.Annotate(err, "while copying to local archive file")
}

func (c *createCommand) create(client APIClient, apiVersion int) (*params.BackupsMetadataResult, string, error) {
	result, err := client.Create(c.Notes, c.KeepCopy, c.NoDownload)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	copyFrom := result.ID

	if apiVersion >= 2 {
		copyFrom = result.Filename
	}

	return result, copyFrom, err
}
