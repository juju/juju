// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

// State represents a type for interacting with the underlying state.
// Composes both user and permission state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*UserState
	*PermissionState
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		UserState:       NewUserState(factory),
		PermissionState: NewPermissionState(factory, logger),
	}
}
