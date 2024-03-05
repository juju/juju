// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const (
	NotSet = notset
)

var (
	NewAPIClient = &newAPIClient
	NewGetAPI    = &getAPI
)

type CreateCommand struct {
	*createCommand
}

type DownloadCommand struct {
	*downloadCommand
}

func NewCreateCommandForTest(store jujuclient.ClientStore) (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommandForTest(store jujuclient.ClientStore) (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &DownloadCommand{c}
}
