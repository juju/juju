// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const (
	NotSet          = notset
	DownloadWarning = downloadWarning
)

var (
	NewAPIClient = &newAPIClient
	NewGetAPI    = &getAPI
	GetArchive   = &getArchive
)

type CreateCommand struct {
	*createCommand
}

type DownloadCommand struct {
	*downloadCommand
}

type RestoreCommand struct {
	*restoreCommand
}

func NewCreateCommandForTest(store jujuclient.ClientStore) (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.SetClientStore(store)
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommandForTest(store jujuclient.ClientStore) (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &DownloadCommand{c}
}

func NewListCommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &listCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func NewShowCommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &showCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func NewUploadCommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &uploadCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func NewRemoveCommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &removeCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func NewRestoreCommandForTest(
	store jujuclient.ClientStore,
) (cmd.Command, *RestoreCommand) {
	c := &restoreCommand{}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &RestoreCommand{c}
}
