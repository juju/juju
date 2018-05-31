// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRestoreCommand returns a command used to restore a backup.
func NewRestoreCommand() cmd.Command {
	return modelcmd.Wrap(&restoreCommand{})
}

// restoreCommand is a subcommand of backups that implement the restore behavior
// it is invoked with "juju restore-backup".
type restoreCommand struct {
	CommandBase
	Filename string
	BackupId string
}

// RestoreAPI is used to invoke various API calls.
type RestoreAPI interface {
	// Close is taken from io.Closer.
	Close() error

	// Restore is taken from backups.Client.
	Restore(backupId string, newClient backups.ClientConnection) error

	// RestoreReader is taken from backups.Client.
	RestoreReader(r io.ReadSeeker, meta *params.BackupsMetadataResult, newClient backups.ClientConnection) error
}

var restoreDoc = `
Restores the Juju state database backup that was previously created with
"juju create-backup", returning an existing controller to a previous state.

Note: Only the database will be restored.  Juju will not change the existing
environment to match the restored database, e.g. no units, relations, nor
machines will be added or removed during the restore process.

If the provided state cannot be restored, this command will fail with
an explanation.
`

// Info returns the content for --help.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore-backup",
		Purpose: "Restore from a backup archive to the existing controller.",
		Args:    "",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

// SetFlags handles known option flags.
func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.Filename, "file", "", "Provide a file to be used as the backup")
	f.StringVar(&c.BackupId, "id", "", "Provide the name of the backup to be restored")
}

// Init is where the preconditions for this commands can be checked.
func (c *restoreCommand) Init(args []string) error {
	if c.Filename == "" && c.BackupId == "" {
		return errors.Errorf("you must specify either a file or a backup id.")
	}
	if c.Filename != "" && c.BackupId != "" {
		return errors.Errorf("you must specify either a file or a backup id but not both.")
	}

	if c.Filename != "" {
		var err error
		c.Filename, err = filepath.Abs(c.Filename)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *restoreCommand) NewClient() (*backups.Client, error) {
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backupsClient, ok := client.(*backups.Client)
	if !ok {
		return nil, errors.Errorf("invalid client for backups")
	}
	return backupsClient, nil
}

// Run is the entry point for this command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}

	var archive ArchiveReader
	var meta *params.BackupsMetadataResult
	target := c.BackupId
	if c.Filename != "" {
		// Read archive specified by the Filename;
		// we'll need the info later regardless if
		// we need it now to rebootstrap.
		target = c.Filename
		var err error
		archive, meta, err = getArchive(c.Filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer archive.Close()
	}

	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	// We have a backup client, now use the relevant method
	// to restore the backup.
	if c.Filename != "" {
		err = client.RestoreReader(archive, meta, c.NewClient)
	} else {
		err = client.Restore(c.BackupId, c.NewClient)
	}
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "restore from %q completed\n", target)
	return nil
}
