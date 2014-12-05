// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/state/backups"
)

const (
	notset          = backups.FilenamePrefix + "<date>-<time>.tar.gz"
	downloadWarning = "WARNING: downloading backup archives is recommended; " +
		"backups stored remotely are not guaranteed to be available"
)

const createDoc = `
"create" requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored remotely by juju.

The --download option may be used without the --filename option.  In
that case, the backup archive will be stored in the current working
directory with a name matching juju-backup-<date>-<time>.tar.gz.

WARNING: Remotely stored backups will be lost when the environment is
destroyed.  Furthermore, the remotely backup is not guaranteed to be
available.

Therefore, you should use the --download or --filename options, or use
"juju backups download", to get a local copy of the backup archive.
This local copy can then be used to restore an environment even if that
environment was already destroyed or is otherwise unavailable.
`

// CreateCommand is the sub-command for creating a new backup.
type CreateCommand struct {
	CommandBase
	// Quiet indicates that the full metadata should not be dumped.
	Quiet bool
	// NoDownload means the backups archive should not be downloaded.
	NoDownload bool
	// Filename is where the backup should be downloaded.
	Filename string
	// Notes is the custom message to associated with the new backup.
	Notes string
}

// Info implements Command.Info.
func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "[<notes>]",
		Purpose: "create a backup",
		Doc:     createDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *CreateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Quiet, "quiet", false, "do not print the metadata")
	f.BoolVar(&c.NoDownload, "no-download", false, "do not download the archive")
	f.StringVar(&c.Filename, "filename", notset, "download to this file")
}

// Init implements Command.Init.
func (c *CreateCommand) Init(args []string) error {
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
func (c *CreateCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	result, err := client.Create(c.Notes)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.Quiet {
		if c.NoDownload {
			fmt.Fprintln(ctx.Stderr, downloadWarning)
		}
		c.dumpMetadata(ctx, result)
	}

	fmt.Fprintln(ctx.Stdout, result.ID)

	// Handle download.
	filename := c.decideFilename(ctx, c.Filename, result.Started)
	if filename != "" {
		if err := c.download(ctx, result.ID, filename); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *CreateCommand) decideFilename(ctx *cmd.Context, filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	if c.NoDownload {
		return ""
	}

	// Downloading but no filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

func (c *CreateCommand) download(ctx *cmd.Context, id string, filename string) error {
	fmt.Fprintln(ctx.Stdout, "downloading to "+filename)

	// TODO(ericsnow) lp-1399722 This needs further investigation:
	// There is at least anecdotal evidence that we cannot use an API
	// client for more than a single request. So we use a new client
	// for download.
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	archive, err := client.Download(id)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	outfile, err := os.Create(filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, archive)
	return errors.Trace(err)
}
