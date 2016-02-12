// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/cmd/modelcmd"
)

func newRestoreCommand() cmd.Command {
	return modelcmd.Wrap(&restoreCommand{})
}

// restoreCommand is a subcommand of backups that implement the restore behavior
// it is invoked with "juju backups restore".
type restoreCommand struct {
	CommandBase
	filename string
	backupId string
}

var restoreDoc = `
Restores a backup that was previously created with "juju create-backup".

This command expects a controller to have been bootstrapped, and then
arranges for it to be restored to the state captured in the specified
backup. As part of the restore, all known instances are configured to
connect to the new controller.

If the provided state cannot be restored, this command will fail with
an appropriate message.  For instance, if the existing bootstrap
instance is already running then the command will fail with a message
to that effect.
`

// Info returns the content for --help.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore",
		Purpose: "restore from a backup archive to a new controller",
		Args:    "",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

// SetFlags handles known option flags.
func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.filename, "file", "", "provide a file to be used as the backup.")
	f.StringVar(&c.backupId, "id", "", "provide the name of the backup to be restored.")
}

// Init is where the preconditions for this commands can be checked.
func (c *restoreCommand) Init(args []string) error {
	if c.filename == "" && c.backupId == "" {
		return errors.Errorf("you must specify either a file or a backup id.")
	}
	if c.filename != "" && c.backupId != "" {
		return errors.Errorf("you must specify either a file or a backup id but not both.")
	}
	var err error
	if c.filename != "" {
		c.filename, err = filepath.Abs(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// runRestore will implement the actual calls to the different Client parts
// of restore.
func (c *restoreCommand) runRestore(ctx *cmd.Context) error {
	client, closer, err := c.newClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	var target string
	var rErr error
	if c.filename != "" {
		target = c.filename
		archive, meta, err := getArchive(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer archive.Close()

		rErr = client.RestoreReader(archive, meta, c.newClient)
	} else {
		target = c.backupId
		rErr = client.Restore(c.backupId, c.newClient)
	}
	if rErr != nil {
		return errors.Trace(rErr)
	}

	fmt.Fprintf(ctx.Stdout, "restore from %q completed\n", target)
	return nil
}

func (c *restoreCommand) newClient() (*backups.Client, func() error, error) {
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	backupsClient, ok := client.(*backups.Client)
	if !ok {
		return nil, nil, errors.Errorf("invalid client for backups")
	}
	return backupsClient, client.Close, nil
}

// Run is the entry point for this command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	return c.runRestore(ctx)
}
