// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"github.com/juju/juju/overlord/logstate"
	"github.com/juju/juju/overlord/state"
)

type LogManager interface {
	StateManager
	AppendLines(state.Txn, []logstate.Line) error
}
