// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"crypto/sha1"
	"fmt"
	"io"
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

The backup archive is always downloaded to the local machine: the
controller creates the archive and streams it back in the same
request, and nothing is kept on the controller once the request ends.
The archive is verified against the recorded checksum before the
download is considered complete; if verification fails, the corrupt
archive is kept locally under a ".corrupt" suffix for inspection and
the backup must be created again. An interrupted transfer leaves no
archive on either side: re-run the command to create the backup
again.

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

	result, archive, err := client.Create(ctx, c.Notes)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	if result.Checksum == "" {
		// The controller always records a checksum for the archive, so
		// an empty checksum means a malformed response. Fail closed
		// rather than write an archive that cannot be verified.
		return errors.Errorf(
			"controller returned no checksum for the backup, refusing to download it unverified")
	}

	filename := c.decideFilename(c.Filename, result.Started)
	if err := c.writeArchive(archive, result.Checksum, filename); err != nil {
		return errors.Trace(err)
	}
	// Print the metadata only after the archive is verified and
	// written, so a scripted consumer never sees a plausible
	// report for a backup that was not delivered.
	if !c.quiet {
		fmt.Fprintln(ctx.Stdout, c.metadata(&result))
	}
	ctx.Infof("Downloaded to %v", filename)
	return nil
}

func (c *createCommand) decideFilename(filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	// No filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

// writeArchive streams the archive into archiveFilename, hashing it
// along the way, and verifies the received bytes against the recorded
// checksum. An incomplete transfer leaves no partial file behind; a
// checksum mismatch keeps the corrupt archive under a ".corrupt"
// suffix for inspection.
func (c *createCommand) writeArchive(stream io.Reader, checksum, archiveFilename string) error {
	archive, err := c.Filesystem().Create(archiveFilename)
	if err != nil {
		return errors.Annotatef(err, "while creating local archive file %v", archiveFilename)
	}

	// The checksum is the base64-encoded SHA-1 sum of the archive as
	// recorded when it was created; hash while streaming so the archive
	// is only read once.
	hasher := hash.NewHashingWriter(archive, sha1.New())
	_, copyErr := io.Copy(hasher, stream)
	// Close the archive before handling either failure, so that an
	// incomplete archive can be removed even on platforms that cannot
	// remove open files.
	closeErr := archive.Close()
	if copyErr != nil || closeErr != nil {
		// The archive is incomplete: remove it so it cannot be
		// mistaken for a complete backup. The removal is best effort:
		// the copy or close failure is the more useful error.
		_ = c.Filesystem().RemoveAll(archiveFilename)
		if copyErr != nil {
			return errors.Annotatef(copyErr, "while copying to local archive file %v", archiveFilename)
		}
		return errors.Annotatef(closeErr, "while closing local archive file %v", archiveFilename)
	}
	if hasher.Base64Sum() != checksum {
		// Keep the corrupt archive under a suffix so the operator can
		// inspect the damage, rather than silently removing it.
		corruptName := archiveFilename + ".corrupt"
		if rerr := c.Filesystem().Rename(archiveFilename, corruptName); rerr != nil {
			return errors.Errorf("checksum mismatch for downloaded backup %q", archiveFilename)
		}
		return errors.Errorf(
			"checksum mismatch for downloaded backup %q (renamed to %q for inspection)",
			archiveFilename, corruptName)
	}
	return nil
}
