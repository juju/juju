// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewTestStatusHistoryCommand(api HistoryAPI) cmd.Command {
	return &statusHistoryCommand{api: api}
}

func NewTestStatusCommand(statusapi statusAPI, clock Clock) cmd.Command {
	return modelcmd.Wrap(
		&statusCommand{statusAPI: statusapi, clock: clock})
}
