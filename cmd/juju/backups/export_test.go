// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
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

func NewCreateCommandForTest(store jujuclient.ClientStore) (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &CreateCommand{c}
}
