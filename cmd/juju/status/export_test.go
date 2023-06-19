// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewStatusHistoryCommandForTest(api HistoryAPI) cmd.Command {
	return &statusHistoryCommand{api: api}
}

func NewStatusCommandForTest(store jujuclient.ClientStore, statusapi statusAPI, storageapi storage.StorageListAPI, clock Clock) cmd.Command {
	cmd := &statusCommand{statusAPI: statusapi, storageAPI: storageapi, clock: clock}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
