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

func NewCreateCommandForTest() (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommandForTest() (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &DownloadCommand{c}
}

func NewListCommandForTest() cmd.Command {
	c := &listCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewShowCommandForTest() cmd.Command {
	c := &showCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewUploadCommandForTest() cmd.Command {
	c := &uploadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewRemoveCommandForTest() cmd.Command {
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
		newAPIClientFunc: func() (RestoreAPI, error) {
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
