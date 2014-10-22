// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const (
	notset           = "<not set>"
	filenameTemplate = "juju-backup-%04d%02d%02d-%02d%%02d%%02d%"
)

const createDoc = `
"create" requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored in juju and
will be lost when the environment is destroyed.
`

// CreateCommand is the sub-command for creating a new backup.
type CreateCommand struct {
	CommandBase
	// Quiet indicates that the full metadata should not be dumped.
	Quiet bool
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
	f.StringVar(&c.Filename, "download", notset, "download the archive")
}

// Init implements Command.Init.
func (c *CreateCommand) Init(args []string) error {
	notes, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Notes = notes
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
		c.dumpMetadata(ctx, result)
	}

	fmt.Fprintln(ctx.Stdout, result.ID)

	if c.Filename != notset {
		filename := c.Filename
		if filename == "" {
			y, m, d := result.Started.Date()
			H, M, S := result.Started.Clock()
			filename = fmt.Sprintf(filenameTemplate, y, m, d, H, M, S)
		}
		c.download(ctx, filename)
	}

	return nil
}

func (c *CreateCommand) download(ctx *cmd.Context, filename string) error {
	fmt.Fprintln(ctx.Stdout, "downloading to "+c.Filename)

	archive, err := client.Download(result.ID)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	outfile, err := os.Create(c.Filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer outfile.Close()

	_, err := io.Copy(outfile, archive)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
