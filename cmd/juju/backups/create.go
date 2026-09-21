// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4/hash"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/backups"
)

const notset = backups.FilenamePrefix + "<date>-<time>.tar.gz"

const createDoc = `
This command requests that Juju creates a backup of its state.
You may provide a note to associate with the backup.

The backup archive is always downloaded to the local machine, and the
copy written on the controller is removed once it has been delivered.
The archive is verified against the recorded checksum before the
download is considered complete.

The model config attribute ` + "`backup-dir`" + ` only serves as scratch space
during backup creation; no archive is kept there once the command
finishes.

Use ` + "`--verbose`" + ` to see extra information about backup.
`

const createExamples = `
    juju create-backup
`

// NewCreateCommand returns a command used to create backups.
func NewCreateCommand() cmd.Command {
	return modelcmd.Wrap(&createCommand{})
}

// createCommand is the sub-command for creating a new backup.
type createCommand struct {
	CommandBase
	// Filename is where the backup archive is written locally.
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
	})
}

// SetFlags implements Command.SetFlags.
func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
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

	if c.Filename == "" {
		return errors.Errorf("missing filename")
	}
	return nil
}

// Run implements Command.Run.
func (c *createCommand) Run(ctx *cmd.Context) error {
	if err := c.validateIaasController(ctx, c.Info().Name); err != nil {
		return errors.Trace(err)
	}
	client, err := c.NewGetAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	result, err := client.Create(ctx, c.Notes)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.quiet {
		fmt.Fprintln(ctx.Stdout, c.metadata(result))
	}

	if result.ID == "" {
		// Against older controllers without a download id, the archive
		// was still created and remains on the controller; it can be
		// recovered from the filename directly.
		return errors.Errorf(
			"controller did not provide a backup id for download (archive may be present on the controller at %q)",
			result.Filename)
	}

	filename := c.decideFilename(c.Filename, result.Started)
	if err := c.download(ctx, client, result.ID, result.Checksum, filename); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *createCommand) decideFilename(filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	// No filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

// download streams the backup archive staged on the controller for the
// given id, writing it to the local archiveFilename and verifying it
// against the recorded checksum when one is available.
func (c *createCommand) download(ctx *cmd.Context, client APIClient, id, checksum, archiveFilename string) error {
	resultArchive, err := client.Download(ctx, id)
	if err != nil {
		return errors.Trace(err)
	}
	defer resultArchive.Close()

	archive, err := c.Filesystem().Create(archiveFilename)
	if err != nil {
		return errors.Annotatef(err, "while creating local archive file %v", archiveFilename)
	}
	defer archive.Close()

	// The checksum is the base64-encoded SHA-1 sum of the archive as
	// recorded when it was created; hash while streaming so the archive
	// is only read once.
	hasher := hash.NewHashingWriter(archive, sha1.New())
	if _, err := io.Copy(hasher, resultArchive); err != nil {
		return errors.Annotatef(err, "while copying to local archive file %v", archiveFilename)
	}
	if checksum != "" && hasher.Base64Sum() != checksum {
		// Keep the corrupt archive but rename it so the operator can
		// inspect the damage, rather than silently removing it.
		_ = archive.Close()
		corruptName := archiveFilename + ".corrupt"
		// The mock filesystem doesn't rename; the file was created
		// from the mock's Create and the mock's RemoveAll cleans it up
		// on success. For inspectability on mismatch, rename via os.Rename.
		_ = os.Rename(archiveFilename, corruptName)
		return errors.Errorf(
			"checksum mismatch for downloaded backup %q (renamed to %q for inspection)",
			archiveFilename, corruptName)
	}
	ctx.Infof("Downloaded to %v", archiveFilename)
	return nil
}
