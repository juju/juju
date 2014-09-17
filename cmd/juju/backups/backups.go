// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"

	"github.com/juju/cmd"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
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
	backupsCmd.Register(envcmd.Wrap(&BackupsCreateCommand{}))
	return &backupsCmd
}

// APIClient represents the backups API client functionality used by
// the backups command.
type APIClient interface {
	io.Closer
	Create(notes string) (*params.BackupsMetadataResult, error)
}

// BackupsCommandBase is the base type for backups sub-commands.
type BackupsCommandBase struct {
	envcmd.EnvCommandBase
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *BackupsCommandBase) NewAPIClient() (APIClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return backups.NewClient(root), nil
}

func (c *BackupsCommandBase) dumpMetadata(ctx *cmd.Context, result *params.BackupsMetadataResult) {
	fmt.Fprintf(ctx.Stdout, "backup ID:       %q\n", result.ID)
	fmt.Fprintf(ctx.Stdout, "started:         %v\n", result.Started)
	fmt.Fprintf(ctx.Stdout, "finished:        %v\n", result.Finished)
	fmt.Fprintf(ctx.Stdout, "checksum:        %q\n", result.Checksum)
	fmt.Fprintf(ctx.Stdout, "checksum format: %q\n", result.ChecksumFormat)
	fmt.Fprintf(ctx.Stdout, "size (B):        %d\n", result.Size)
	fmt.Fprintf(ctx.Stdout, "stored:          %t\n", result.Stored)
	fmt.Fprintf(ctx.Stdout, "notes:           %q\n", result.Notes)

	fmt.Fprintf(ctx.Stdout, "environment ID:  %q\n", result.Environment)
	fmt.Fprintf(ctx.Stdout, "machine ID:      %q\n", result.Machine)
	fmt.Fprintf(ctx.Stdout, "created on host: %q\n", result.Hostname)
	fmt.Fprintf(ctx.Stdout, "juju version:    %v\n", result.Version)
}
