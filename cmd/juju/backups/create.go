// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

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
the backup must be created again. When the controller records its
checksum in a format this client cannot verify (an older controller),
the download fails and no archive is kept. An interrupted transfer
leaves no archive on either side: re-run the command to create the
backup again.

Once the archive is verified and saved, the backup summary is written
beside it as an unsigned YAML assertion file, named by appending
".assert" to the archive filename. The assertion records the archive's
SHA-256 checksum together with the backup metadata. On success the
command prints nothing.

Recovering a controller from a backup needs both files, the archive
and its assertion: see "juju recover <backup-file> <assertion-file>".

The model config attribute ` + "`backup-dir`" + ` only serves as scratch space
during backup creation; no archive is kept there once the command
finishes.

Only one backup archive can be created at a time across the controller.
If another creation is running, retry after it finishes. The creation
lock is released before the completed archive is downloaded.

Each database (the controller database and every model database) is
exported in its own transaction at a slightly different moment: the
archive is not a single point-in-time snapshot of the whole controller.
Create the backup during a quiet window, when models are not being
deployed, destroyed or upgraded, so that concurrent changes cannot
produce an inconsistent archive.
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
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Filename, "filename", notset, "Download to this file")
}

// Init implements Command.Init.
func (c *createCommand) Init(args []string) error {
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
	if err := c.writeArchive(ctx, archive, result.Checksum, result.ChecksumFormat, filename); err != nil {
		return errors.Trace(err)
	}
	// The assertion is written only after the archive is verified and
	// saved, so an assertion never describes an archive that was not
	// delivered.
	data, err := c.metadata(&result)
	if err != nil {
		return errors.Trace(err)
	}
	return c.writeAssertion(data, filename+".assert")
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
// checksum. Verification only applies when the controller reports the
// current checksum format: an older controller records a SHA-1 checksum
// this client cannot verify, so the download fails rather than keep an
// archive whose integrity is unconfirmed. An incomplete transfer leaves
// no partial file behind; a checksum mismatch keeps the corrupt archive
// under a ".corrupt" suffix for inspection.
func (c *createCommand) writeArchive(ctx *cmd.Context, stream io.Reader, checksum, checksumFormat, archiveFilename string) error {
	archive, err := c.Filesystem().Create(archiveFilename)
	if err != nil {
		return errors.Annotatef(err, "while creating local archive file %v", archiveFilename)
	}

	// The checksum is the SHA-256 sum of the archive as recorded when
	// it was created, in the canonical prefixed form; hash while
	// streaming so the archive is only read once.
	hasher := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(archive, hasher), stream)
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
	if checksumFormat != backups.ChecksumFormatSHA256 {
		// The checksum cannot be interpreted in an unknown format, so
		// the archive's integrity is unconfirmed: remove it rather
		// than keep a backup that cannot be verified.
		_ = c.Filesystem().RemoveAll(archiveFilename)
		return errors.Errorf(
			"controller reported checksum format %q; the downloaded archive cannot be verified",
			checksumFormat)
	}
	if !strings.EqualFold(backups.ChecksumSHA256(hex.EncodeToString(hasher.Sum(nil))), backups.ChecksumSHA256(checksum)) {
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

// writeAssertion saves the marshalled assertion summary beside the
// verified archive. A short write or a failed close fails the command
// and removes the partial file, so no assertion survives for an
// archive it does not describe; the verified archive itself is kept.
func (c *createCommand) writeAssertion(data []byte, filename string) error {
	file, err := c.Filesystem().Create(filename)
	if err != nil {
		return errors.Annotatef(err, "while creating local assertion file %v", filename)
	}
	n, writeErr := file.Write(data)
	if writeErr == nil && n < len(data) {
		// A truncated assertion could not be parsed, so treat a
		// short write as a failure.
		writeErr = io.ErrShortWrite
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		// The assertion is incomplete: remove it so it cannot be
		// mistaken for a usable one. The removal is best effort: the
		// write or close failure is the more useful error.
		_ = c.Filesystem().RemoveAll(filename)
		if writeErr != nil {
			return errors.Annotatef(writeErr, "while writing local assertion file %v", filename)
		}
		return errors.Annotatef(closeErr, "while closing local assertion file %v", filename)
	}
	return nil
}
