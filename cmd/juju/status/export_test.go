// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewStatusHistoryCommandForTest(clients []HistoryAPI) cmd.Command {
	return &statusHistoryCommand{clients: clients}
}

func NewStatusCommandForTest(store jujuclient.ClientStore, statusapi statusAPI, clock Clock) cmd.Command {
	cmd := &statusCommand{statusAPI: statusapi, clock: clock}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
