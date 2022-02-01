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

	// ActionByTag returns one action by tag.
	// If no action is found, then it returns a NotFound error.
	ActionByTag(state.Txn, names.ActionTag) (*actionstate.Action, error)

	// ActionsByName returns a slice of actions that have the same name.
	ActionsByName(state.Txn, string) ([]*actionstate.Action, error)

	// AddAction adds an action, returning the given action.
	AddAction(state.Txn, names.Tag, string, string, map[string]interface{}) (*actionstate.Action, error)

	// CancelActionByTag cancels an action by tag and returns the action that
	// was canceled.
	CancelActionByTag(state.Txn, names.ActionTag) (*actionstate.Action, error)
}
