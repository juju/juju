// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import "github.com/juju/cmd"

func NewTestStatusHistoryCommand(api HistoryAPI) cmd.Command {
	return &statusHistoryCommand{api: api}
}
