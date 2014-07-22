// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
)

var backupsDoc = `
"juju backups" is used to manage backups of the state of a juju environment.
`

const backupsPurpose = "create, manage, and restore backups of juju's state"

// BackupsCommand is the top-level command wrapping all backups functionality.
type BackupsCommand struct {
	cmd.SuperCommand
}

// NewBackupsCommand returns a new BackupsCommand.
func NewBackupsCommand() cmd.Command {
	backupsCmd := BackupsCommand{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "backups",
				Doc:         backupsDoc,
				UsagePrefix: "juju",
				Purpose:     backupsPurpose,
			},
		),
	}
	return &backupsCmd
}

type backups interface {
	io.Closer
	Create(notes string) (*params.BackupsMetadataResult, error)
}

// BackupsCommandBase is the base type for backups sub-commands.
type BackupsCommandBase struct {
	envcmd.EnvCommandBase
}

func (c *BackupsCommandBase) client() (backups, error) {
	client, err := juju.NewAPIClientFromName(c.ConnectionName())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	return client.Backups(), nil
}

func (c *BackupsCommandBase) dumpMetadata(ctx *cmd.Context, result *params.BackupsMetadataResult) {
	fmt.Fprintf(ctx.Stdout, "backup ID:       %s\n", result.ID)
	fmt.Fprintf(ctx.Stdout, "started:         %v\n", result.Started)
	fmt.Fprintf(ctx.Stdout, "finished:        %v\n", result.Finished)
	fmt.Fprintf(ctx.Stdout, "checksum:        %s\n", result.Checksum)
	fmt.Fprintf(ctx.Stdout, "checksum format: %s\n", result.ChecksumFormat)
	fmt.Fprintf(ctx.Stdout, "size (B):        %d\n", result.Size)
	fmt.Fprintf(ctx.Stdout, "stored:          %u\n", result.Stored)
	fmt.Fprintf(ctx.Stdout, "notes:           %s\n", result.Notes)

	fmt.Fprintf(ctx.Stdout, "environment ID:  %s\n", result.Environment)
	fmt.Fprintf(ctx.Stdout, "machine ID:      %s\n", result.Machine)
	fmt.Fprintf(ctx.Stdout, "created on host: %s\n", result.Hostname)
	fmt.Fprintf(ctx.Stdout, "juju version:    %v\n", result.Version)
}
