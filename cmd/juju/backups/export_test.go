// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
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

func NewRestoreCommandForTest(
	store jujuclient.ClientStore,
	api RestoreAPI,
	getArchive func(string) (ArchiveReader, *params.BackupsMetadataResult, error),
	getEnviron func(string, *params.BackupsMetadataResult) (environs.Environ, error),
) cmd.Command {
	c := &restoreCommand{
		getArchiveFunc: getArchive,
		getEnvironFunc: getEnviron,
		newAPIFunc: func() (RestoreAPI, error) {
			return api, nil
		}}
	if getEnviron == nil {
		c.getEnvironFunc = func(controllerNme string, meta *params.BackupsMetadataResult) (environs.Environ, error) {
			return c.getEnviron(controllerNme, meta)
		}
	}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}
