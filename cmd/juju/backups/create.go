// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/backups"
)

const (
	notset          = backups.FilenamePrefix + "<date>-<time>.tar.gz"
	downloadWarning = "--no-download flag is DEPRECATED."
)

const createDoc = `
This command requests that Juju creates a backup of its state.
You may provide a note to associate with the backup.

By default, the backup archive and associated metadata are downloaded.

Use ` + "`--no-download`" + ` to avoid getting a local copy of the backup downloaded
at the end of the backup process. In this case it is recommended that the
model config attribute ` + "`backup-dir`" + ` be set to point to a path where the
backup archives should be stored long term. This could be a remotely mounted
filesystem; the same path must exist on each controller if using HA.

Use ` + "`--verbose`" + ` to see extra information about backup.

To access remote backups stored on the controller, see ` + "`juju download-backup`" + `.
`

const createExamples = `
    juju create-backup
    juju create-backup --no-download
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
}

// Info implements Command.Info.
func (c *createCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "create-backup",
		Args:     "[<notes>]",
		Purpose:  "Create a backup.",
		Doc:      createDoc,
		Examples: createExamples,
		SeeAlso: []string{
			"download-backup",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.NoDownload, "no-download", false, "Do not download the archive. DEPRECATED.")
	f.StringVar(&c.Filename, "filename", notset, "Download to this file")
	c.fs = f
}

// Init implements Command.Init.
func (c *createCommand) Init(args []string) error {
	if err := c.CommandBase.Init(args); err != nil {
		return err
	}
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
	if err := c.validateIaasController(c.Info().Name); err != nil {
		return errors.Trace(err)
	}
	client, err := c.NewGetAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if c.NoDownload {
		ctx.Warningf(downloadWarning)
	}

	metadataResult, copyFrom, err := c.create(client)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.quiet {
		fmt.Fprintln(ctx.Stdout, c.metadata(metadataResult))
	}

	if c.NoDownload {
		ctx.Infof("Remote backup stored on the controller as %v", metadataResult.Filename)
	} else {
		filename := c.decideFilename(c.Filename, metadataResult.Started)
		if err := c.download(ctx, client, copyFrom, filename); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *createCommand) decideFilename(filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	// Downloading but no filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

func (c *createCommand) download(ctx *cmd.Context, client APIClient, copyFrom string, archiveFilename string) error {
	resultArchive, err := client.Download(copyFrom)
	if err != nil {
		return errors.Trace(err)
	}
	defer resultArchive.Close()

	archive, err := c.Filesystem().Create(archiveFilename)
	if err != nil {
		return errors.Annotatef(err, "while creating local archive file %v", archiveFilename)
	}
	defer archive.Close()

	_, err = io.Copy(archive, resultArchive)
	if err != nil {
		return errors.Annotatef(err, "while copying to local archive file %v", archiveFilename)
	}
	ctx.Infof("Downloaded to %v", archiveFilename)
	return nil
}

func (c *createCommand) create(client APIClient) (*params.BackupsMetadataResult, string, error) {
	result, err := client.Create(c.Notes, c.NoDownload)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	copyFrom := result.Filename

	return result, copyFrom, err
}
