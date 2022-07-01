// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachedimages

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/jujuclient"
)

var (
	GetListImagesAPI  = &getListImagesAPI
	GetRemoveImageAPI = &getRemoveImageAPI
)

func NewListCommandForTest(store jujuclient.ClientStore) cmd.Command {
	cmd := &listCommand{}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewRemoveCommandForTest(store jujuclient.ClientStore) cmd.Command {
	cmd := &removeCommand{}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
