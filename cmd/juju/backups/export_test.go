// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

const (
	NotSet          = notset
	DownloadWarning = downloadWarning
)

var (
	NewAPIClient = &newAPIClient

	NewInfoCommand    = newInfoCommand
	NewListCommand    = newListCommand
	NewUploadCommand  = newUploadCommand
	NewRemoveCommand  = newRemoveCommand
	NewRestoreCommand = newRestoreCommand
)

type CreateCommand struct {
	*createCommand
}

type DownloadCommand struct {
	*downloadCommand
}

func NewCreateCommand() (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	return envcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommand() (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	return envcmd.Wrap(c), &DownloadCommand{c}
}
