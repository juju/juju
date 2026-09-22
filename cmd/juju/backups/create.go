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

The backup archive is always downloaded to the local machine. The
archive is verified against the recorded checksum before the download
is considered complete, and an interrupted transfer is retried
automatically with the same id.

The staged copy on the controller is removed once the archive has
been fully served; a partial transfer leaves it staged so the retry
can fetch it again. If verification fails, the corrupt archive is
kept locally under a ".corrupt" suffix for inspection and the backup
must be created again.

The model config attribute ` + "`backup-dir`" + ` only serves as scratch space
during backup creation; no archive is kept there once the command
finishes. On an HA controller the archive is staged on the controller
machine that served the request and downloaded over the same API
connection, so a retry that reaches a different controller machine
will not find it; point ` + "`backup-dir`" + ` at a filesystem shared by all
controller machines if download retries must survive reconnection.

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

	if result.Checksum == "" {
		// A controller that returns a download id always records a
		// checksum for the archive too, so an empty checksum means a
		// malformed response. Fail closed rather than download an
		// archive that cannot be verified.
		return errors.Errorf(
			"controller returned no checksum for backup %q, refusing to download it unverified",
			result.ID)
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

// maxDownloadAttempts bounds how many times a failed archive download
// is retried before the command gives up.
const maxDownloadAttempts = 3

var (
	// errChecksumMismatch reports that the bytes received for a
	// backup archive do not match the checksum recorded when the
	// backup was created.
	errChecksumMismatch = errors.ConstError("checksum mismatch")

	// errLocalArchiveFile reports that the archive could not be
	// written to the local machine. Fetching the archive again cannot
	// fix a local filesystem problem, so download attempts that fail
	// this way are not retried.
	errLocalArchiveFile = errors.ConstError("local archive file failure")
)

// download streams the backup archive staged on the controller for the
// given id, writing it to the local archiveFilename and verifying it
// against the recorded checksum. An interrupted transfer is retried
// with the same id: a partial transfer leaves the archive staged, so
// the same id streams the whole archive again on retry. A checksum
// mismatch is not retried: it is only detected after a fully-served
// transfer, by which point the controller has removed the archive.
func (c *createCommand) download(ctx *cmd.Context, client APIClient, id, checksum, archiveFilename string) error {
	var err error
	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		if attempt > 1 {
			ctx.Infof("Retrying backup download (attempt %d of %d): %v",
				attempt, maxDownloadAttempts, err)
		}
		if err = c.fetchArchive(ctx, client, id, checksum, archiveFilename); err == nil {
			ctx.Infof("Downloaded to %v", archiveFilename)
			return nil
		}
		if !retriableDownloadFailure(err) {
			break
		}
	}
	if errors.Is(err, errChecksumMismatch) {
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
	return errors.Trace(err)
}

// retriableDownloadFailure reports whether a failed attempt may
// succeed if the archive is fetched again with the same id. Failures
// writing the archive locally, an id that is no longer staged on the
// controller, and a checksum mismatch are permanent: a mismatch is
// only detected after a fully-served transfer, and the controller
// removes the archive once it has been fully served, so a retry can
// only 404. Anything else is worth another attempt.
func retriableDownloadFailure(err error) bool {
	if errors.Is(err, errLocalArchiveFile) || errors.Is(err, errChecksumMismatch) {
		return false
	}
	return !errors.IsNotFound(err)
}

// fetchArchive performs one download attempt: it streams the staged
// archive for id into archiveFilename, hashing it along the way, and
// reports errChecksumMismatch when the received bytes do not match
// the recorded checksum.
func (c *createCommand) fetchArchive(ctx *cmd.Context, client APIClient, id, checksum, archiveFilename string) error {
	resultArchive, err := client.Download(ctx, id)
	if err != nil {
		return errors.Trace(err)
	}
	defer resultArchive.Close()

	archive, err := c.Filesystem().Create(archiveFilename)
	if err != nil {
		// The failure is local, so mark it as such to keep the
		// download from being retried.
		return fmt.Errorf("while creating local archive file %v: %w%w",
			archiveFilename, err, errors.Hide(errLocalArchiveFile))
	}

	// The checksum is the base64-encoded SHA-1 sum of the archive as
	// recorded when it was created; hash while streaming so the archive
	// is only read once.
	hasher := hash.NewHashingWriter(archive, sha1.New())
	_, copyErr := io.Copy(hasher, resultArchive)
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
		return errChecksumMismatch
	}
	return nil
}
