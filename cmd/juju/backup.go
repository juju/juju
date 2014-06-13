// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/state/api"
)

type backupClient interface {
	Backup(backupFilePath string) (string, error)
}

type BackupCommand struct {
	/* XXX Shuld this really be an env-specific command? */
	envcmd.EnvCommandBase
	Filename string
}

var backupDoc = fmt.Sprintf(`
This command will generate a backup of juju's state as a local gzipped tar
file (.tar.gz).  If no filename is provided, one is generated with a name
like this:

  %s

where <timestamp> is the timestamp of when the backup is generated.

The filename of the generated archive will be printed upon success.
`, fmt.Sprintf(api.BACKUP_FILENAME, "<timestamp>"))

func (c *BackupCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "backup",
		Args:    "[filename]",
		Purpose: "create a backup of juju's state",
		Doc:     backupDoc,
	}
}

func (c *BackupCommand) Init(args []string) error {
	filename, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		c.Filename = filename
	}
	return err
}

func (c *BackupCommand) run(ctx *cmd.Context, client backupClient) error {
	filename, err := client.Backup(c.Filename)
	if err != nil {
		return err
	}

	fmt.Fprintln(ctx.Stdout, filename)
	return nil
}

func (c *BackupCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	return c.run(ctx, client)
}
