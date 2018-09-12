// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewTestStatusHistoryCommand(api HistoryAPI) cmd.Command {
	return &statusHistoryCommand{api: api}
}

func NewTestStatusCommand(api statusAPI, clock Clock) cmd.Command {
	return modelcmd.Wrap(
		&statusCommand{api: api, clock: clock})
}
