// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/database"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State represents a type for interacting with the underlying state.
// Composes both user and permission state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*UserState
	*PermissionState
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger Logger) *State {
	return &State{
		UserState:       NewUserState(factory),
		PermissionState: NewPermissionState(factory, logger),
	}
}
