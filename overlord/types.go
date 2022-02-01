// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/overlord/actionstate"
	"github.com/juju/juju/overlord/logstate"
	"github.com/juju/juju/overlord/state"
)

type LogManager interface {
	StateManager
	AppendLines(state.Txn, []logstate.Line) error
}

type ActionManager interface {
	StateManager
	ActionByTag(state.Txn, names.ActionTag) (*actionstate.Action, error)
}
