// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

const (
	NotSet          = notset
	DownloadWarning = downloadWarning
)

var (
	NewAPIClient = &newAPIClient
)

type CreateCommand struct {
	*createCommand
}

type DownloadCommand struct {
	*downloadCommand
}

func NewCreateCommand() (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommand() (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &DownloadCommand{c}
}

func NewListCommand() cmd.Command {
	c := &listCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewInfoCommand() cmd.Command {
	c := &infoCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewUploadCommand() cmd.Command {
	c := &uploadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewRemoveCommand() cmd.Command {
	c := &removeCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewRestoreCommand() cmd.Command {
	c := &restoreCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}
